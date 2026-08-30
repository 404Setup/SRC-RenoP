/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

const (
	abbreviatedMetadataType = "application/vnd.npm.install-v1+json"
	maxPackumentSize        = 16 << 20
)

var abbreviatedVersionFields = map[string]struct{}{
	"name": {}, "version": {}, "deprecated": {}, "dependencies": {}, "acceptDependencies": {},
	"optionalDependencies": {}, "devDependencies": {}, "bundleDependencies": {}, "bundledDependencies": {},
	"peerDependencies": {}, "peerDependenciesMeta": {}, "bin": {}, "directories": {}, "dist": {},
	"engines": {}, "_hasShrinkwrap": {}, "hasInstallScript": {}, "funding": {}, "cpu": {}, "os": {},
}

func npmError(c fiber.Ctx, status int, code, reason string) error {
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(status).JSON(registryError{Error: code, Reason: reason})
}

func revisionString(pkg *core.NPMPackage) string {
	if pkg == nil {
		return "0-0"
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d",
		pkg.Repository, pkg.Name, pkg.Revision, pkg.UpdatedAt)))
	return fmt.Sprintf("%d-%s", pkg.Revision, hex.EncodeToString(digest[:8]))
}

func abbreviatedManifest(manifest map[string]any) map[string]any {
	filtered := make(map[string]any, len(abbreviatedVersionFields))
	for key, value := range manifest {
		if _, allowed := abbreviatedVersionFields[key]; allowed {
			filtered[key] = value
		}
	}
	return filtered
}

func packument(details *core.NPMPackageDetails, baseURL string, abbreviated bool) (map[string]any, error) {
	if details == nil || details.Package == nil {
		return nil, core.ErrNPMPackageNotFound
	}
	pkg := details.Package
	versions := make(map[string]any)
	times := map[string]any{
		"created":  time.UnixMilli(pkg.CreatedAt).UTC().Format(time.RFC3339Nano),
		"modified": time.UnixMilli(pkg.UpdatedAt).UTC().Format(time.RFC3339Nano),
	}
	var latestManifest map[string]any
	latestVersion := details.DistTags["latest"]
	for _, version := range details.Versions {
		if version == nil || version.Unpublished {
			continue
		}
		manifest := make(map[string]any)
		if err := json.Unmarshal([]byte(version.ManifestJSON), &manifest); err != nil {
			return nil, fmt.Errorf("decode npm version %s: %w", version.Version, err)
		}
		setCanonicalDist(manifest, baseURL, pkg.Repository, pkg.Name, version.Version,
			version.TarballPath, version.Shasum, version.Integrity, version.Publisher, version.Size)
		if version.Deprecated != "" {
			manifest["deprecated"] = version.Deprecated
		} else {
			delete(manifest, "deprecated")
		}
		if version.Version == latestVersion {
			latestManifest = manifest
		}
		if abbreviated {
			manifest = abbreviatedManifest(manifest)
		}
		versions[version.Version] = manifest
		times[version.Version] = time.UnixMilli(version.CreatedAt).UTC().Format(time.RFC3339Nano)
	}
	result := map[string]any{
		"name": pkg.Name, "dist-tags": details.DistTags, "versions": versions,
	}
	if abbreviated {
		result["modified"] = time.UnixMilli(pkg.UpdatedAt).UTC().Format(time.RFC3339Nano)
		return result, nil
	}
	result["_id"] = pkg.Name
	result["_rev"] = revisionString(pkg)
	result["description"] = pkg.Description
	result["time"] = times
	maintainers := make([]map[string]string, 0, len(details.Members))
	for _, member := range details.Members {
		if member != nil && member.Level >= core.NPMPermissionPublish {
			maintainers = append(maintainers, map[string]string{"name": member.Username})
		}
	}
	result["maintainers"] = maintainers
	for _, key := range []string{
		"author", "bugs", "contributors", "homepage", "keywords", "license", "readme",
		"readmeFilename", "repository",
	} {
		if value, exists := latestManifest[key]; exists {
			result[key] = value
		}
	}
	return result, nil
}

func packumentNotModified(c fiber.Ctx, etag string, updatedAt int64) bool {
	if strings.TrimSpace(c.Get(fiber.HeaderIfNoneMatch)) == etag {
		return true
	}
	ifModifiedSince := strings.TrimSpace(c.Get(fiber.HeaderIfModifiedSince))
	if ifModifiedSince == "" {
		return false
	}
	parsed, err := http.ParseTime(ifModifiedSince)
	return err == nil && time.UnixMilli(updatedAt).Truncate(time.Second).Compare(parsed) <= 0
}

func mirroredPackumentNeedsRefresh(pkg *core.NPMPackage, repo *config.Repository) bool {
	if pkg == nil {
		return true
	}
	if !pkg.Mirrored {
		return false
	}
	persist, ttl := repo.GetCacheConfig()
	return !persist || ttl > 0 && time.Since(time.UnixMilli(pkg.UpdatedAt)) > time.Duration(ttl)*time.Second
}

func refreshMirroredPackumentOnce(c fiber.Ctx, state *core.AppState, repo *config.Repository,
	packageName string) error {
	if state == nil || state.Inner == nil || state.Inner.InFlightDownloads == nil || state.GetDB() == nil {
		return core.ErrDatabaseUnavailable
	}
	key := "npm-packument:" + repo.Name + "/" + packageName
	refresh := state.Inner.InFlightDownloads.AcquirePath(key)
	succeeded := false
	defer func() { state.Inner.InFlightDownloads.UnlockPath(key, refresh, succeeded) }()
	pkg, err := state.GetDB().GetNPMPackage(repo.Name, packageName)
	if err != nil {
		return err
	}
	if !mirroredPackumentNeedsRefresh(pkg, repo) {
		succeeded = true
		return nil
	}
	if err := refreshMirroredPackument(c.Context(), state, repo, packageName); err != nil {
		return err
	}
	succeeded = true
	return nil
}

func servePackument(c fiber.Ctx, state *core.AppState, repo *config.Repository, packageName string) error {
	if state.GetDB() == nil {
		return npmError(c, fiber.StatusServiceUnavailable, "database unavailable", "npm metadata is unavailable")
	}
	pkg, err := state.GetDB().GetNPMPackage(repo.Name, packageName)
	if err != nil {
		return npmError(c, fiber.StatusInternalServerError, "metadata failure", "failed to load npm package")
	}
	refresh := mirroredPackumentNeedsRefresh(pkg, repo)
	if refresh && len(repo.Mirrors) > 0 {
		if mirrorErr := refreshMirroredPackumentOnce(c, state, repo, packageName); mirrorErr == nil {
			pkg, err = state.GetDB().GetNPMPackage(repo.Name, packageName)
		} else if pkg == nil && !errors.Is(mirrorErr, core.ErrNPMPackageNotFound) {
			return npmError(c, fiber.StatusBadGateway, "upstream failure", "failed to load npm package from upstream")
		}
	}
	if err != nil || pkg == nil || pkg.Archived {
		return npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
	}
	allowed, accessErr := CanReadPackage(state, auth.GetUser(c), repo, packageName)
	if accessErr != nil {
		return npmError(c, fiber.StatusServiceUnavailable, "metadata failure", "npm package access is unavailable")
	}
	if !allowed {
		return npmError(c, fiber.StatusNotFound, "not_found", "npm package was not found")
	}
	details, err := state.GetDB().GetNPMPackageDetails(repo.Name, packageName, auth.GetUser(c).Username)
	if err != nil || details == nil {
		return npmError(c, fiber.StatusInternalServerError, "metadata failure", "failed to build npm package metadata")
	}
	abbreviated := strings.Contains(strings.ToLower(c.Get(fiber.HeaderAccept)), abbreviatedMetadataType)
	document, err := packument(details, c.BaseURL(), abbreviated)
	if err != nil {
		return npmError(c, fiber.StatusInternalServerError, "metadata failure", "failed to build npm package metadata")
	}
	etag := `"npm-` + revisionString(pkg) + `-` + fmt.Sprint(abbreviated) + `"`
	modified := time.UnixMilli(pkg.UpdatedAt).UTC().Format(http.TimeFormat)
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderLastModified, modified)
	c.Set(fiber.HeaderVary, "Accept, Authorization")
	if pkg.Private {
		c.Set(fiber.HeaderCacheControl, "private, no-store")
	} else {
		c.Set(fiber.HeaderCacheControl, "public, max-age=0, must-revalidate")
	}
	if packumentNotModified(c, etag, pkg.UpdatedAt) {
		return c.SendStatus(fiber.StatusNotModified)
	}
	contentType := fiber.MIMEApplicationJSON
	if abbreviated {
		contentType = abbreviatedMetadataType
	}
	c.Set(fiber.HeaderContentType, contentType+"; charset=utf-8")
	if c.Method() == fiber.MethodHead {
		return c.SendStatus(fiber.StatusOK)
	}
	body, err := json.Marshal(document)
	if err != nil {
		return npmError(c, fiber.StatusInternalServerError, "metadata failure", "failed to encode npm package metadata")
	}
	return c.Status(fiber.StatusOK).Send(body)
}

func refreshMirroredPackument(ctx context.Context, state *core.AppState, repo *config.Repository,
	packageName string) error {
	var lastErr error
	for index := range repo.Mirrors {
		mirror := repo.Mirrors[index]
		if allowed, _ := mirror.IsArtifactAllowedFor(config.RepositoryFormatNPM, packageName); !allowed {
			continue
		}
		base := strings.TrimRight(strings.TrimSpace(mirror.URL), "/")
		if base == "" {
			continue
		}
		client, err := proxy.ClientForMirror(&mirror, state.Inner.Config.Load().Proxy)
		if err != nil || client == nil {
			lastErr = errors.Join(lastErr, err)
			continue
		}
		timeout := time.Duration(max(mirror.TimeoutSecs, 1)) * time.Second
		requestContext, cancel := context.WithTimeout(ctx, timeout)
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet,
			base+"/"+escapedPackageName(packageName), nil)
		if err != nil {
			cancel()
			lastErr = errors.Join(lastErr, err)
			continue
		}
		request.Header.Set(fiber.HeaderAccept, fiber.MIMEApplicationJSON)
		if mirror.Authorization != nil {
			if err := mirror.Authorization.Apply(request); err != nil {
				cancel()
				lastErr = errors.Join(lastErr, err)
				continue
			}
		}
		select {
		case state.Inner.ProxyClientSemaphore <- struct{}{}:
		case <-requestContext.Done():
			cancel()
			lastErr = errors.Join(lastErr, requestContext.Err())
			continue
		}
		response, err := client.Do(request)
		if err != nil {
			<-state.Inner.ProxyClientSemaphore
			cancel()
			lastErr = errors.Join(lastErr, err)
			continue
		}
		if response.StatusCode == http.StatusNotFound {
			utils.DiscardHTTPBody(response.Body, response.ContentLength)
			<-state.Inner.ProxyClientSemaphore
			cancel()
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices ||
			response.ContentLength > maxPackumentSize {
			utils.DiscardHTTPBody(response.Body, response.ContentLength)
			<-state.Inner.ProxyClientSemaphore
			cancel()
			lastErr = errors.Join(lastErr, fmt.Errorf("npm mirror %d returned status %d", index+1, response.StatusCode))
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxPackumentSize+1))
		closeErr := response.Body.Close()
		<-state.Inner.ProxyClientSemaphore
		cancel()
		if readErr != nil || closeErr != nil || len(body) > maxPackumentSize {
			lastErr = errors.Join(lastErr, readErr, closeErr)
			continue
		}
		pkg, versions, tags, parseErr := parseMirroredPackument(repo.Name, packageName, body,
			mirror.Authorization != nil)
		if parseErr != nil {
			lastErr = errors.Join(lastErr, parseErr)
			continue
		}
		return state.GetDB().RecordNPMMirrorPublication(pkg, versions, tags)
	}
	if lastErr != nil {
		return lastErr
	}
	return core.ErrNPMPackageNotFound
}

func parseMirroredPackument(repository, expectedName string, body []byte, authenticated bool) (
	*core.NPMPackage, []*core.NPMVersion, map[string]string, error,
) {
	var document struct {
		Name        string                     `json:"name"`
		Description string                     `json:"description"`
		Access      string                     `json:"access"`
		DistTags    map[string]string          `json:"dist-tags"`
		Time        map[string]string          `json:"time"`
		Versions    map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, nil, nil, errors.New("upstream npm packument is invalid")
	}
	packageName, valid := NormalizePackageName(document.Name)
	if !valid || packageName != expectedName || len(document.Versions) > 5000 {
		return nil, nil, nil, errors.New("upstream npm packument has invalid package metadata")
	}
	now := time.Now().UnixMilli()
	createdAt := now
	if parsed, err := time.Parse(time.RFC3339Nano, document.Time["created"]); err == nil {
		createdAt = parsed.UnixMilli()
	}
	updatedAt := now
	if parsed, err := time.Parse(time.RFC3339Nano, document.Time["modified"]); err == nil {
		updatedAt = parsed.UnixMilli()
	}
	versions := make([]*core.NPMVersion, 0, len(document.Versions))
	knownVersions := make(map[string]struct{}, len(document.Versions))
	for versionName, raw := range document.Versions {
		if !validNPMVersion(versionName) || len(raw) == 0 || len(raw) > maxStoredManifestJSON {
			continue
		}
		manifest := make(map[string]any)
		if err := json.Unmarshal(raw, &manifest); err != nil {
			continue
		}
		manifestName, _ := manifest["name"].(string)
		normalized, valid := NormalizePackageName(manifestName)
		manifestVersion, _ := manifest["version"].(string)
		if !valid || normalized != packageName || manifestVersion != versionName {
			continue
		}
		dist, _ := manifest["dist"].(map[string]any)
		shasum, _ := dist["shasum"].(string)
		integrity, _ := dist["integrity"].(string)
		deprecated, _ := manifest["deprecated"].(string)
		tarballPath := canonicalTarballPath(packageName, versionName)
		setCanonicalDist(manifest, "", repository, packageName, versionName, tarballPath,
			strings.ToLower(shasum), integrity, "mirror", 0)
		serialized, err := json.Marshal(manifest)
		if err != nil || len(serialized) > maxStoredManifestJSON {
			continue
		}
		publishedAt := createdAt
		if parsed, err := time.Parse(time.RFC3339Nano, document.Time[versionName]); err == nil {
			publishedAt = parsed.UnixMilli()
		}
		versions = append(versions, &core.NPMVersion{
			Repository: repository, Package: packageName, Version: versionName,
			ManifestJSON: string(serialized), Publisher: "mirror", TarballPath: tarballPath,
			Shasum: strings.ToLower(shasum), Integrity: integrity, Deprecated: deprecated,
			Mirrored: true, CreatedAt: publishedAt,
		})
		knownVersions[versionName] = struct{}{}
	}
	if len(versions) == 0 {
		return nil, nil, nil, errors.New("upstream npm packument contains no usable versions")
	}
	tags := make(map[string]string)
	for tag, version := range document.DistTags {
		if validNPMTag(tag) {
			if _, known := knownVersions[version]; known {
				tags[strings.ToLower(tag)] = version
			}
		}
	}
	latest := tags["latest"]
	return &core.NPMPackage{
		Repository: repository, Name: packageName, Description: document.Description,
		LatestVersion: latest, Private: authenticated || strings.EqualFold(document.Access, "restricted"),
		Mirrored: true, PublishEnabled: false, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, versions, tags, nil
}
