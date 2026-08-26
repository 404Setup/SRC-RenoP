/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"renop/internal/core"
	"renop/internal/service/index"
	"renop/internal/service/storage"
	"renop/internal/utils"
)

const (
	maxMavenPOMBytes            = 2 << 20
	maxMavenVersionFiles        = 64
	maxMavenProjectLicenses     = 16
	maxMavenProjectDevelopers   = 32
	maxMavenProjectDependencies = 128
)

var mavenChecksumCompanions = []struct {
	suffix string
	label  string
}{
	{suffix: ".md5", label: "MD5"},
	{suffix: ".sha1", label: "SHA-1"},
	{suffix: ".sha256", label: "SHA-256"},
	{suffix: ".sha512", label: "SHA-512"},
}

type mavenPOMCoordinate struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type mavenPOMLicense struct {
	Name string `xml:"name"`
	URL  string `xml:"url"`
}

type mavenPOMDeveloper struct {
	ID              string   `xml:"id"`
	Name            string   `xml:"name"`
	URL             string   `xml:"url"`
	Organization    string   `xml:"organization"`
	OrganizationURL string   `xml:"organizationUrl"`
	Roles           []string `xml:"roles>role"`
}

type mavenPOMDependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Type       string `xml:"type"`
	Classifier string `xml:"classifier"`
	Optional   string `xml:"optional"`
}

type mavenPOMProject struct {
	XMLName       xml.Name            `xml:"project"`
	ModelVersion  string              `xml:"modelVersion"`
	Parent        *mavenPOMCoordinate `xml:"parent"`
	Name          string              `xml:"name"`
	Description   string              `xml:"description"`
	Packaging     string              `xml:"packaging"`
	URL           string              `xml:"url"`
	InceptionYear string              `xml:"inceptionYear"`
	Organization  struct {
		Name string `xml:"name"`
		URL  string `xml:"url"`
	} `xml:"organization"`
	SCM struct {
		URL string `xml:"url"`
	} `xml:"scm"`
	IssueManagement struct {
		System string `xml:"system"`
		URL    string `xml:"url"`
	} `xml:"issueManagement"`
	Licenses             []mavenPOMLicense    `xml:"licenses>license"`
	Developers           []mavenPOMDeveloper  `xml:"developers>developer"`
	Dependencies         []mavenPOMDependency `xml:"dependencies>dependency"`
	DependencyManagement struct {
		Dependencies []mavenPOMDependency `xml:"dependencies>dependency"`
	} `xml:"dependencyManagement"`
}

func boundedMavenMetadataText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}

func normalizeMavenFileTimestamp(value int64) int64 {
	if value <= 0 {
		return 0
	}
	if value > 10_000_000_000_000 {
		return value / 1_000_000
	}
	return value
}

func isMavenCompanionFile(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".asc") {
		return true
	}
	for _, companion := range mavenChecksumCompanions {
		if strings.HasSuffix(lower, companion.suffix) {
			return true
		}
	}
	return false
}

func mavenFileKind(name, artifactID, version string) (extension, classifier string) {
	extension = strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	prefix := artifactID + "-" + version
	if strings.HasPrefix(stem, prefix+"-") {
		classifier = boundedMavenMetadataText(strings.TrimPrefix(stem, prefix+"-"), 255)
	}
	return extension, classifier
}

func inspectMavenVersionFiles(fileIndex *index.FileIndex, versionDir, artifactID string,
	version *core.MavenVersion) string {
	if fileIndex == nil || version == nil {
		return ""
	}
	children := fileIndex.GetChildren(versionDir)
	slices.Sort(children)
	indexed := make(map[string]index.FileInfo, len(children))
	for _, name := range children {
		if info, ok := fileIndex.GetFileInfo(filepath.Join(versionDir, name)); ok {
			indexed[name] = info
		}
	}
	files := make([]*core.MavenVersionFile, 0, min(len(indexed), maxMavenVersionFiles))
	pomName := ""
	for _, name := range children {
		info, ok := indexed[name]
		if !ok || isMavenCompanionFile(name) {
			continue
		}
		extension, classifier := mavenFileKind(name, artifactID, version.Version)
		checksums := make([]string, 0, len(mavenChecksumCompanions))
		for _, companion := range mavenChecksumCompanions {
			if _, exists := indexed[name+companion.suffix]; exists {
				checksums = append(checksums, companion.label)
			}
		}
		_, signed := indexed[name+".asc"]
		version.FileCount++
		version.TotalFileSize += max(info.Size, 0)
		if signed {
			version.SignedFileCount++
		}
		modifiedAt := normalizeMavenFileTimestamp(info.ModTime)
		version.LastModified = max(version.LastModified, modifiedAt)
		if extension == "pom" && (pomName == "" || classifier == "") {
			pomName = name
		}
		if len(files) < maxMavenVersionFiles {
			files = append(files, &core.MavenVersionFile{
				Name: name, Extension: extension, Classifier: classifier,
				Size: max(info.Size, 0), ModifiedAt: modifiedAt, Signed: signed, Checksums: checksums,
			})
		}
	}
	version.Files = files
	version.FilesTruncated = version.FileCount > len(files)
	return pomName
}

func openMavenMetadataFile(path string) (io.ReadCloser, int64, error) {
	if storage.IsS3Enabled(path) {
		reader, info, err := storage.DownloadFromS3(utils.GetS3Key(path))
		return reader, info.Size, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	return file, info.Size(), nil
}

func projectCoordinate(source *mavenPOMCoordinate) *core.MavenProjectCoordinate {
	if source == nil {
		return nil
	}
	coordinate := &core.MavenProjectCoordinate{
		GroupID:    boundedMavenMetadataText(source.GroupID, 255),
		ArtifactID: boundedMavenMetadataText(source.ArtifactID, 255),
		Version:    boundedMavenMetadataText(source.Version, 255),
	}
	if coordinate.GroupID == "" && coordinate.ArtifactID == "" && coordinate.Version == "" {
		return nil
	}
	return coordinate
}

func projectMetadataFromPOM(project *mavenPOMProject) *core.MavenProjectMetadata {
	if project == nil {
		return nil
	}
	metadata := &core.MavenProjectMetadata{
		ModelVersion:           boundedMavenMetadataText(project.ModelVersion, 64),
		Name:                   boundedMavenMetadataText(project.Name, 512),
		Description:            boundedMavenMetadataText(project.Description, 4000),
		Packaging:              boundedMavenMetadataText(project.Packaging, 128),
		URL:                    boundedMavenMetadataText(project.URL, 2048),
		InceptionYear:          boundedMavenMetadataText(project.InceptionYear, 32),
		Parent:                 projectCoordinate(project.Parent),
		OrganizationName:       boundedMavenMetadataText(project.Organization.Name, 512),
		OrganizationURL:        boundedMavenMetadataText(project.Organization.URL, 2048),
		SCMURL:                 boundedMavenMetadataText(project.SCM.URL, 2048),
		IssueManagementSystem:  boundedMavenMetadataText(project.IssueManagement.System, 128),
		IssueManagementURL:     boundedMavenMetadataText(project.IssueManagement.URL, 2048),
		ManagedDependencyCount: len(project.DependencyManagement.Dependencies),
	}
	if metadata.Packaging == "" {
		metadata.Packaging = "jar"
	}
	for _, license := range project.Licenses[:min(len(project.Licenses), maxMavenProjectLicenses)] {
		entry := &core.MavenProjectLicense{
			Name: boundedMavenMetadataText(license.Name, 512),
			URL:  boundedMavenMetadataText(license.URL, 2048),
		}
		if entry.Name != "" || entry.URL != "" {
			metadata.Licenses = append(metadata.Licenses, entry)
		}
	}
	for _, developer := range project.Developers[:min(len(project.Developers), maxMavenProjectDevelopers)] {
		entry := &core.MavenProjectDeveloper{
			ID:              boundedMavenMetadataText(developer.ID, 255),
			Name:            boundedMavenMetadataText(developer.Name, 512),
			URL:             boundedMavenMetadataText(developer.URL, 2048),
			Organization:    boundedMavenMetadataText(developer.Organization, 512),
			OrganizationURL: boundedMavenMetadataText(developer.OrganizationURL, 2048),
		}
		for _, role := range developer.Roles[:min(len(developer.Roles), 8)] {
			if role = boundedMavenMetadataText(role, 128); role != "" {
				entry.Roles = append(entry.Roles, role)
			}
		}
		if entry.ID != "" || entry.Name != "" || entry.URL != "" || entry.Organization != "" {
			metadata.Developers = append(metadata.Developers, entry)
		}
	}
	dependencyLimit := min(len(project.Dependencies), maxMavenProjectDependencies)
	for _, dependency := range project.Dependencies[:dependencyLimit] {
		entry := &core.MavenProjectDependency{
			GroupID:    boundedMavenMetadataText(dependency.GroupID, 255),
			ArtifactID: boundedMavenMetadataText(dependency.ArtifactID, 255),
			Version:    boundedMavenMetadataText(dependency.Version, 255),
			Scope:      boundedMavenMetadataText(dependency.Scope, 64),
			Type:       boundedMavenMetadataText(dependency.Type, 64),
			Classifier: boundedMavenMetadataText(dependency.Classifier, 255),
			Optional:   strings.EqualFold(strings.TrimSpace(dependency.Optional), "true"),
		}
		if entry.GroupID == "" || entry.ArtifactID == "" {
			continue
		}
		if entry.Scope == "" {
			entry.Scope = "compile"
		}
		if entry.Type == "" {
			entry.Type = "jar"
		}
		metadata.Dependencies = append(metadata.Dependencies, entry)
	}
	metadata.DependenciesTruncated = len(project.Dependencies) > dependencyLimit
	return metadata
}

func loadMavenProjectMetadata(path string) (*core.MavenProjectMetadata, error) {
	reader, size, err := openMavenMetadataFile(path)
	if err != nil {
		return nil, err
	}
	if size < 0 || size > maxMavenPOMBytes {
		return nil, errors.Join(fmt.Errorf("maven POM size %d exceeds the metadata limit", size), reader.Close())
	}
	limited := &io.LimitedReader{R: reader, N: maxMavenPOMBytes + 1}
	var project mavenPOMProject
	decodeErr := xml.NewDecoder(limited).Decode(&project)
	closeErr := reader.Close()
	if decodeErr != nil {
		return nil, errors.Join(fmt.Errorf("decode Maven POM metadata: %w", decodeErr), closeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close Maven POM metadata: %w", closeErr)
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("maven POM exceeds the metadata limit")
	}
	return projectMetadataFromPOM(&project), nil
}

func enrichMavenArtifactDetails(state *core.AppState, repository string,
	details *core.MavenArtifactDetails) error {
	if state == nil || state.Inner == nil || state.Inner.FileIndex == nil || details == nil || details.Artifact == nil {
		return nil
	}
	cfg := state.Inner.Config.Load()
	if cfg == nil {
		return nil
	}
	artifact := details.Artifact
	artifactRoot := filepath.Join(cfg.StoragePath, repository,
		filepath.FromSlash(strings.ReplaceAll(artifact.GroupID, ".", "/")), artifact.ArtifactID)
	if !utils.IsSubPath(filepath.Join(cfg.StoragePath, repository), artifactRoot) {
		return fmt.Errorf("maven artifact metadata path is outside the repository")
	}
	latestPOMPath := ""
	for _, version := range details.Versions {
		versionDir := filepath.Join(artifactRoot, version.Version)
		if !utils.IsSubPath(artifactRoot, versionDir) {
			continue
		}
		pomName := inspectMavenVersionFiles(state.Inner.FileIndex, versionDir, artifact.ArtifactID, version)
		details.FileCount += version.FileCount
		details.TotalFileSize += version.TotalFileSize
		details.SignedFileCount += version.SignedFileCount
		if version.Version == artifact.LatestVersion && pomName != "" {
			latestPOMPath = filepath.Join(versionDir, pomName)
		}
	}
	if latestPOMPath == "" {
		return nil
	}
	project, err := loadMavenProjectMetadata(latestPOMPath)
	if err != nil {
		return err
	}
	details.Project = project
	return nil
}
