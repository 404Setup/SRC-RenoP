/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-json"

	"renop/internal/core"
)

const (
	maxNPMReadmeBytes        = 512 << 10
	maxNPMProjectTextRunes   = 2048
	maxNPMProjectPeople      = 32
	maxNPMProjectKeywords    = 64
	maxNPMProjectFundingURLs = 16
)

func boundedNPMProjectText(value any, maxRunes int) string {
	text, _ := value.(string)
	text = strings.TrimSpace(strings.ReplaceAll(text, "\x00", ""))
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes])
	}
	return text
}

func boundedNPMReadme(value any) string {
	readme, _ := value.(string)
	readme = strings.TrimSpace(strings.ReplaceAll(readme, "\x00", ""))
	if readme == "" || strings.EqualFold(readme, "ERROR: No README data found!") {
		return ""
	}
	if len(readme) <= maxNPMReadmeBytes {
		return readme
	}
	readme = readme[:maxNPMReadmeBytes]
	for !utf8.ValidString(readme) {
		readme = readme[:len(readme)-1]
	}
	return strings.TrimSpace(readme) + "\n\n…"
}

func safeNPMProjectURL(value any) string {
	raw := boundedNPMProjectText(value, maxNPMProjectTextRunes)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "github:") {
		repository := raw[len("github:"):]
		repository = strings.Trim(strings.TrimSuffix(repository, ".git"), "/")
		if repository != "" && !strings.Contains(repository, "..") {
			raw = "https://github.com/" + repository
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "git+") {
		raw = raw[len("git+"):]
	}
	if strings.HasPrefix(strings.ToLower(raw), "git://") {
		raw = "https://" + raw[len("git://"):]
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil ||
		!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	return parsed.String()
}

func npmProjectURL(value any) string {
	if object, ok := value.(map[string]any); ok {
		return safeNPMProjectURL(object["url"])
	}
	return safeNPMProjectURL(value)
}

func npmProjectPerson(value any) *core.NPMProjectPerson {
	person := &core.NPMProjectPerson{}
	switch typed := value.(type) {
	case map[string]any:
		person.Name = boundedNPMProjectText(typed["name"], 255)
		person.Email = boundedNPMProjectText(typed["email"], 320)
		person.URL = safeNPMProjectURL(typed["url"])
	case string:
		remaining := strings.TrimSpace(typed)
		if open, close := strings.LastIndex(remaining, "("), strings.LastIndex(remaining, ")"); open >= 0 && close > open {
			person.URL = safeNPMProjectURL(remaining[open+1 : close])
			remaining = strings.TrimSpace(remaining[:open] + remaining[close+1:])
		}
		if open, close := strings.LastIndex(remaining, "<"), strings.LastIndex(remaining, ">"); open >= 0 && close > open {
			person.Email = boundedNPMProjectText(remaining[open+1:close], 320)
			remaining = strings.TrimSpace(remaining[:open] + remaining[close+1:])
		}
		person.Name = boundedNPMProjectText(remaining, 255)
	}
	if person.Name == "" && person.Email == "" && person.URL == "" {
		return nil
	}
	return person
}

func npmProjectStrings(value any, limit, maxRunes int) []string {
	values := make([]any, 0)
	switch typed := value.(type) {
	case []any:
		values = typed
	case string:
		for _, item := range strings.Split(typed, ",") {
			values = append(values, item)
		}
	}
	result := make([]string, 0, min(len(values), limit))
	seen := make(map[string]struct{}, min(len(values), limit))
	for _, item := range values {
		text := boundedNPMProjectText(item, maxRunes)
		key := strings.ToLower(text)
		if text == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, text)
		if len(result) == limit {
			break
		}
	}
	return result
}

func npmProjectFunding(value any) []string {
	values := []any{value}
	if list, ok := value.([]any); ok {
		values = list
	}
	result := make([]string, 0, min(len(values), maxNPMProjectFundingURLs))
	seen := make(map[string]struct{}, cap(result))
	for _, item := range values {
		link := npmProjectURL(item)
		if link == "" {
			continue
		}
		if _, duplicate := seen[link]; duplicate {
			continue
		}
		seen[link] = struct{}{}
		result = append(result, link)
		if len(result) == maxNPMProjectFundingURLs {
			break
		}
	}
	return result
}

func npmProjectContributors(value any) []core.NPMProjectPerson {
	list, _ := value.([]any)
	result := make([]core.NPMProjectPerson, 0, min(len(list), maxNPMProjectPeople))
	for _, item := range list {
		if person := npmProjectPerson(item); person != nil {
			result = append(result, *person)
		}
		if len(result) == maxNPMProjectPeople {
			break
		}
	}
	return result
}

func selectNPMProjectVersion(details *core.NPMPackageDetails) *core.NPMVersion {
	if details == nil || details.Package == nil {
		return nil
	}
	latest := details.Package.LatestVersion
	for _, version := range details.Versions {
		if version != nil && !version.Unpublished && version.Version == latest {
			return version
		}
	}
	for _, version := range details.Versions {
		if version != nil && !version.Unpublished {
			return version
		}
	}
	return nil
}

func enrichNPMProjectMetadata(details *core.NPMPackageDetails) {
	version := selectNPMProjectVersion(details)
	if version == nil || version.ManifestJSON == "" {
		return
	}
	manifest := make(map[string]any)
	if err := json.Unmarshal([]byte(version.ManifestJSON), &manifest); err != nil {
		return
	}
	project := &core.NPMProjectMetadata{
		Readme:         boundedNPMReadme(manifest["readme"]),
		ReadmeFilename: boundedNPMProjectText(manifest["readmeFilename"], 255),
		License:        boundedNPMProjectText(manifest["license"], 255),
		Homepage:       safeNPMProjectURL(manifest["homepage"]),
		Repository:     npmProjectURL(manifest["repository"]),
		Bugs:           npmProjectURL(manifest["bugs"]),
		Author:         npmProjectPerson(manifest["author"]),
		Contributors:   npmProjectContributors(manifest["contributors"]),
		Maintainers:    npmProjectContributors(manifest["maintainers"]),
		Funding:        npmProjectFunding(manifest["funding"]),
		Keywords:       npmProjectStrings(manifest["keywords"], maxNPMProjectKeywords, 128),
		PackageManager: boundedNPMProjectText(manifest["packageManager"], 255),
	}
	if license, ok := manifest["license"].(map[string]any); ok {
		project.License = boundedNPMProjectText(license["type"], 255)
	}
	if engines, ok := manifest["engines"].(map[string]any); ok {
		project.NodeEngine = boundedNPMProjectText(engines["node"], 255)
	}
	if project.Readme == "" && project.License == "" && project.Homepage == "" && project.Repository == "" &&
		project.Bugs == "" && project.Author == nil && len(project.Contributors) == 0 &&
		len(project.Maintainers) == 0 && len(project.Funding) == 0 &&
		len(project.Keywords) == 0 && project.NodeEngine == "" && project.PackageManager == "" {
		return
	}
	details.Project = project
}
