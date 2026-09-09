/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"encoding/xml"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

// FilterMetadata returns a viewer-specific projection without changing shared cached metadata.
func FilterMetadata(state *core.AppState, user *config.User, repository, metadataPath string, metadata *config.Metadata) (*config.Metadata, error) {
	base := path.Dir(strings.ReplaceAll(metadataPath, `\`, "/"))
	paths := []string{metadataPath}
	versions := []string{}
	if metadata.Versioning != nil {
		if metadata.Versioning.Versions != nil {
			versions = metadata.Versioning.Versions.Version
		}
		for _, version := range versions {
			paths = append(paths, base+"/"+version)
		}
	}
	fieldStart := len(paths)
	if metadata.Versioning != nil {
		for _, field := range []*string{metadata.Versioning.Latest, metadata.Versioning.Release} {
			value := ""
			if field != nil {
				value = *field
			}
			paths = append(paths, base+"/"+value)
		}
	}
	pluginStart := len(paths)
	if metadata.Plugins != nil {
		for _, plugin := range metadata.Plugins.Plugin {
			name := ""
			if plugin.ArtifactID != nil {
				name = *plugin.ArtifactID
			}
			paths = append(paths, base+"/"+name)
		}
	}
	visible, err := VisibleMetadataPaths(state, user, repository, paths)
	if err != nil {
		return nil, err
	}
	if !visible[0] {
		return nil, fiber.ErrNotFound
	}
	filtered := *metadata
	if metadata.Versioning != nil {
		versioning := *metadata.Versioning
		filtered.Versioning = &versioning
		hidden := make(map[string]bool)
		kept := make([]string, 0, len(versions))
		latest, release := "", ""
		for i, version := range versions {
			if !visible[i+1] {
				hidden[core.ResourceLockVersionKey("maven", version)] = true
				continue
			}
			kept = append(kept, version)
			if latest == "" || utils.CompareVersions(version, latest) > 0 {
				latest = version
			}
			if !strings.HasSuffix(strings.ToUpper(version), "-SNAPSHOT") && (release == "" || utils.CompareVersions(version, release) > 0) {
				release = version
			}
		}
		if versioning.Versions != nil {
			versioning.Versions = &config.Versions{Version: kept}
		}
		for i, field := range []struct {
			value       **string
			replacement string
		}{{&versioning.Latest, latest}, {&versioning.Release, release}} {
			if *field.value != nil && !visible[fieldStart+i] {
				*field.value = nil
				if field.replacement != "" {
					replacement := field.replacement
					*field.value = &replacement
				}
			}
		}
		if len(hidden) > 0 {
			versioning.LastUpdated = nil
		}
	}
	if metadata.Plugins != nil {
		filtered.Plugins = &config.Plugins{}
		for i, plugin := range metadata.Plugins.Plugin {
			if visible[pluginStart+i] {
				filtered.Plugins.Plugin = append(filtered.Plugins.Plugin, plugin)
			}
		}
	}
	return &filtered, nil
}

// HandleReadLocks denies file bytes to every role and exposes only permitted XML metadata.
func HandleReadLocks(c fiber.Ctx, state *core.AppState, repo *config.Repository, storagePath, requestPath string) (bool, error) {
	if repo == nil || repo.NormalizedFormat() != config.RepositoryFormatMaven {
		return false, nil
	}
	if state == nil || state.GetDB() == nil {
		return true, fiber.ErrServiceUnavailable
	}
	name := strings.ToLower(path.Base(requestPath))
	lockPath, descendants := requestPath, false
	if strings.HasPrefix(name, "maven-metadata.xml") {
		lockPath, descendants = path.Dir(requestPath), true
	}
	locks, err := state.GetDB().GetMavenPathLocks(repo.Name, lockPath, descendants)
	if err != nil {
		return true, fiber.ErrServiceUnavailable
	}
	if len(locks) == 0 {
		return false, nil
	}
	c.Locals(core.LockedArtifactPathLocal, requestPath)
	if !core.ReadLocked(locks) {
		return false, nil
	}
	if name != "maven-metadata.xml" {
		return true, c.SendStatus(fiber.StatusNotFound)
	}
	localPath := filepath.Join(storagePath, repo.Name, filepath.FromSlash(requestPath))
	if !utils.IsSubPath(filepath.Join(storagePath, repo.Name), localPath) {
		return true, fiber.ErrBadRequest
	}
	reader, size, err := openMavenMetadataFile(localPath)
	if err != nil {
		return true, fiber.ErrNotFound
	}
	defer reader.Close()
	if size > maxMavenPOMBytes {
		return true, fiber.ErrRequestEntityTooLarge
	}
	limited := &io.LimitedReader{R: reader, N: maxMavenPOMBytes + 1}
	var metadata config.Metadata
	if xml.NewDecoder(limited).Decode(&metadata) != nil || limited.N <= 0 {
		return true, fiber.ErrNotFound
	}
	filtered, err := FilterMetadata(state, auth.GetUser(c), repo.Name, requestPath, &metadata)
	if err != nil {
		return true, err
	}
	encoded, err := xml.Marshal(struct {
		XMLName xml.Name `xml:"metadata"`
		*config.Metadata
	}{Metadata: filtered})
	if err != nil {
		return true, fiber.ErrInternalServerError
	}
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationXML)
	c.Set(fiber.HeaderCacheControl, "no-store")
	return true, c.Send(append([]byte(xml.Header), encoded...))
}
