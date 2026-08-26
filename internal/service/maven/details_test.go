/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func TestEnrichMavenArtifactDetailsProjectsFilesAndIntegrity(t *testing.T) {
	storagePath := t.TempDir()
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()

	const (
		repository = "releases"
		groupID    = "com.example"
		artifactID = "demo"
		version    = "1.2.3"
	)
	versionDir := filepath.Join(storagePath, repository, "com", "example", artifactID, version)
	require.NoError(t, os.MkdirAll(versionDir, 0755))
	pom := `<project>
  <modelVersion>4.0.0</modelVersion>
  <parent><groupId>com.example</groupId><artifactId>parent</artifactId><version>2</version></parent>
  <name>Demo Library</name><description>Bounded project metadata.</description><packaging>jar</packaging>
  <url>https://example.com/demo</url><inceptionYear>2024</inceptionYear>
  <organization><name>Example Org</name><url>https://example.com</url></organization>
  <licenses><license><name>Apache-2.0</name><url>https://www.apache.org/licenses/LICENSE-2.0</url></license></licenses>
  <developers><developer><id>alice</id><name>Alice</name><roles><role>maintainer</role></roles></developer></developers>
  <scm><url>https://github.com/example/demo</url></scm>
  <issueManagement><system>GitHub</system><url>https://github.com/example/demo/issues</url></issueManagement>
  <dependencies>
    <dependency><groupId>org.example</groupId><artifactId>api</artifactId><version>3</version></dependency>
    <dependency><groupId>org.example</groupId><artifactId>runtime</artifactId><scope>runtime</scope><optional>true</optional></dependency>
  </dependencies>
  <dependencyManagement><dependencies><dependency><groupId>org.example</groupId><artifactId>bom</artifactId></dependency></dependencies></dependencyManagement>
</project>`
	files := map[string]string{
		"demo-1.2.3.pom":              pom,
		"demo-1.2.3.pom.sha256":       "checksum",
		"demo-1.2.3.jar":              "binary",
		"demo-1.2.3.jar.asc":          "signature",
		"demo-1.2.3.jar.sha512":       "checksum",
		"demo-1.2.3-sources.jar":      "sources",
		"demo-1.2.3-sources.jar.sha1": "checksum",
	}
	modTime := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC).UnixNano()
	for name, content := range files {
		path := filepath.Join(versionDir, name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		state.Inner.FileIndex.EnsureParentDirs(path)
		state.Inner.FileIndex.InsertFile(path, index.FileInfo{Size: int64(len(content)), ModTime: modTime})
	}

	details := &core.MavenArtifactDetails{
		Artifact: &core.MavenArtifact{
			Repository: repository, Domain: groupID, GroupID: groupID, ArtifactID: artifactID,
			LatestVersion: version,
		},
		Versions: []*core.MavenVersion{{
			Repository: repository, GroupID: groupID, ArtifactID: artifactID, Version: version,
		}},
	}
	require.NoError(t, enrichMavenArtifactDetails(state, repository, details))
	require.NotNil(t, details.Project)
	assert.Equal(t, "Demo Library", details.Project.Name)
	assert.Equal(t, "com.example:parent:2", details.Project.Parent.GroupID+":"+
		details.Project.Parent.ArtifactID+":"+details.Project.Parent.Version)
	assert.Equal(t, "Example Org", details.Project.OrganizationName)
	assert.Equal(t, "https://github.com/example/demo", details.Project.SCMURL)
	assert.Equal(t, 1, details.Project.ManagedDependencyCount)
	require.Len(t, details.Project.Dependencies, 2)
	assert.Equal(t, "compile", details.Project.Dependencies[0].Scope)
	assert.Equal(t, "jar", details.Project.Dependencies[0].Type)
	assert.True(t, details.Project.Dependencies[1].Optional)

	require.Len(t, details.Versions, 1)
	indexedVersion := details.Versions[0]
	assert.Equal(t, 3, indexedVersion.FileCount)
	assert.Equal(t, 1, indexedVersion.SignedFileCount)
	assert.Equal(t, normalizeMavenFileTimestamp(modTime), indexedVersion.LastModified)
	assert.Equal(t, indexedVersion.FileCount, details.FileCount)
	assert.Equal(t, indexedVersion.TotalFileSize, details.TotalFileSize)
	require.Len(t, indexedVersion.Files, 3)
	jar := indexedVersion.Files[0]
	for _, file := range indexedVersion.Files {
		if file.Name == "demo-1.2.3.jar" {
			jar = file
			break
		}
	}
	assert.True(t, jar.Signed)
	assert.Equal(t, []string{"SHA-512"}, jar.Checksums)
}

func TestMavenProjectMetadataCapsPublishedCollections(t *testing.T) {
	project := &mavenPOMProject{}
	for index := 0; index < maxMavenProjectDependencies+7; index++ {
		project.Dependencies = append(project.Dependencies, mavenPOMDependency{
			GroupID: "com.example", ArtifactID: fmt.Sprintf("dependency-%d", index),
		})
	}
	for index := 0; index < maxMavenProjectLicenses+3; index++ {
		project.Licenses = append(project.Licenses, mavenPOMLicense{Name: fmt.Sprintf("license-%d", index)})
	}
	for index := 0; index < maxMavenProjectDevelopers+3; index++ {
		project.Developers = append(project.Developers, mavenPOMDeveloper{Name: fmt.Sprintf("developer-%d", index)})
	}

	metadata := projectMetadataFromPOM(project)
	require.NotNil(t, metadata)
	assert.Len(t, metadata.Dependencies, maxMavenProjectDependencies)
	assert.True(t, metadata.DependenciesTruncated)
	assert.Len(t, metadata.Licenses, maxMavenProjectLicenses)
	assert.Len(t, metadata.Developers, maxMavenProjectDevelopers)
}
