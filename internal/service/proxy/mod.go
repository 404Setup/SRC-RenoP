/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/cargo"
	"renop/internal/service/index"
	"renop/internal/service/status"
	"renop/internal/utils"
)

var (
	IsS3Enabled               func(repo *config.Repository) bool
	UploadToS3                func(repo *config.Repository, localPath string, s3Key string) error
	UploadStreamToS3          func(repo *config.Repository, s3Key string, reader io.Reader, size int64, contentType string) error
	OnArtifactStored          func(localPath string)
	OnArtifactStoredWithState func(state *core.AppState, repo *config.Repository, localPath string)
)

// ErrUpstreamProbeUnavailable indicates that at least one applicable mirror
// could not provide an authoritative package-name availability result.
var ErrUpstreamProbeUnavailable = errors.New("upstream mirror availability check failed")

var httpClient = newProxyHTTPClient()

// escapeArtifactPath remains package-local for existing proxy benchmarks and
// tests; Cargo-specific path handling lives in the cargo package.
func escapeArtifactPath(path string) string {
	return cargo.EscapePath(path)
}

const (
	maxProxyArtifactSize = 512 << 20
	maxProxyMetadataSize = 2 << 20
	proxyDiskReserve     = 64 << 20
)

type multiReadCloser struct {
	io.Reader
	io.Closer
}

func notifyArtifactStored(state *core.AppState, repo *config.Repository, localPath string) {
	if OnArtifactStored != nil {
		OnArtifactStored(localPath)
	}
	if OnArtifactStoredWithState != nil {
		OnArtifactStoredWithState(state, repo, localPath)
	}
}

func proxyResponseLimit(repo *config.Repository, path string) int64 {
	if repo != nil && repo.NormalizedFormat() == config.RepositoryFormatCargo {
		return cargo.ResponseLimit(path)
	}
	name := strings.ToLower(filepath.Base(path))
	if name == "maven-metadata.xml" || strings.HasPrefix(name, "maven-metadata.xml.") {
		return maxProxyMetadataSize
	}
	return maxProxyArtifactSize
}

func newProxyHTTPClient() *http.Client {
	transport := utils.DefaultTransport.Clone()
	// Mirror routing is controlled by Settings > Proxy. An empty global
	// selection means direct connection, regardless of process proxy env vars.
	transport.Proxy = nil
	return &http.Client{
		Transport:     transport,
		CheckRedirect: checkMirrorRedirect,
	}
}

func checkMirrorRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}

	previous := via[len(via)-1]
	if strings.EqualFold(previous.URL.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return errors.New("mirror redirect must not downgrade HTTPS")
	}
	if previous.Header.Get("Authorization") != "" && !sameURLOrigin(previous.URL, req.URL) {
		return errors.New("mirror redirect must not forward credentials across origins")
	}
	return nil
}

func sameURLOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || !strings.EqualFold(a.Scheme, b.Scheme) ||
		!strings.EqualFold(a.Hostname(), b.Hostname()) {
		return false
	}
	return effectiveURLPort(a) == effectiveURLPort(b)
}

func effectiveURLPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(u.Scheme, "http") {
		return "80"
	}
	return ""
}

func mirrorRequestTimeout(timeoutSecs uint64) time.Duration {
	if timeoutSecs == 0 {
		timeoutSecs = config.DefaultMirrorTimeout()
	}
	const maxTimeout = 30 * time.Minute
	if timeoutSecs > uint64(maxTimeout/time.Second) {
		return maxTimeout
	}
	return time.Duration(timeoutSecs) * time.Second
}

func canAllocateProxyDisk(state *core.AppState, bytes uint64) bool {
	if bytes > ^uint64(0)-proxyDiskReserve {
		return false
	}
	return status.CanAllocateDiskSpace(state, bytes+proxyDiskReserve)
}

func saveToDiskAndS3(state *core.AppState, repo *config.Repository, localFilePath string, data []byte) bool {
	defer status.MarkStorageUpdated()
	if IsS3Enabled != nil && IsS3Enabled(repo) && UploadStreamToS3 != nil {
		s3Key := utils.GetS3Key(localFilePath)
		contentType := utils.ContentTypeByExt(filepath.Ext(localFilePath))
		err := UploadStreamToS3(repo, s3Key, bytes.NewReader(data), int64(len(data)), contentType)
		if err == nil {
			state.Inner.FileIndex.EnsureParentDirs(localFilePath)
			state.Inner.FileIndex.InsertFile(localFilePath, index.FileInfo{
				Size:    int64(len(data)),
				ModTime: time.Now().UnixNano(),
			})
			notifyArtifactStored(state, repo, localFilePath)
			return true
		}
		return false
	}

	dir := filepath.Dir(localFilePath)
	_ = os.MkdirAll(dir, 0755)

	uniqueID := uuid.New().String()
	tmpPath := localFilePath + ".tmp." + uniqueID

	err := os.WriteFile(tmpPath, data, 0644)
	if err != nil {
		_ = os.Remove(tmpPath)
		return false
	}

	if err := os.Rename(tmpPath, localFilePath); err == nil {
		state.Inner.FileIndex.EnsureParentDirs(localFilePath)
		state.Inner.FileIndex.InsertFile(localFilePath, index.FileInfo{
			Size:    int64(len(data)),
			ModTime: time.Now().UnixNano(),
		})
		notifyArtifactStored(state, repo, localFilePath)

		return true
	}

	_ = os.Remove(tmpPath)
	return false
}

func ProxyArtifact(state *core.AppState, repo *config.Repository, path string, storagePath string, pathStr string, dl *core.InFlightDownload) (io.ReadCloser, error) {
	shouldNegativeCache := false
	var negativeTtl uint64 = 3600
	networkAttempted := false

	sanitizedPath, ok := utils.SanitizePath(path)
	if !ok {
		state.Inner.InFlightDownloads.UnlockPath(pathStr, dl, false)
		return nil, fiber.ErrBadRequest
	}
	path = sanitizedPath
	var lastBlockedReason string
	var globalProxyConfig config.ProxyConfig
	if state != nil && state.Inner != nil {
		if cfg := state.Inner.Config.Load(); cfg != nil {
			globalProxyConfig = cfg.Proxy
		}
	}
	for _, mirror := range repo.Mirrors {
		if allowed, reason := mirror.IsArtifactAllowedFor(repo.NormalizedFormat(), path); !allowed {
			lastBlockedReason = reason
			continue
		}
		mirrorUrl := cargo.ArtifactURL(repo, mirror, path)
		if mirrorUrl == "" {
			continue
		}

		streamCtx, streamCancel := context.WithTimeout(context.Background(), mirrorRequestTimeout(mirror.TimeoutSecs))
		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, mirrorUrl, nil)
		if err != nil {
			streamCancel()
			continue
		}
		if mirror.Authorization != nil {
			if err := mirror.Authorization.Apply(req); err != nil {
				streamCancel()
				continue
			}
		}

		select {
		case state.Inner.ProxyClientSemaphore <- struct{}{}:
		case <-streamCtx.Done():
			streamCancel()
			continue
		}

		networkAttempted = true
		client, err := clientForMirror(&mirror, globalProxyConfig)
		if err != nil {
			streamCancel()
			<-state.Inner.ProxyClientSemaphore
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			streamCancel()
			<-state.Inner.ProxyClientSemaphore
			continue
		}

		if res.StatusCode == http.StatusNotFound && mirror.NegativeCache {
			shouldNegativeCache = true
			negativeTtl = mirror.CacheTtlSecs
		}

		if res.StatusCode >= 200 && res.StatusCode < 300 {
			localFilePath := filepath.Join(storagePath, repo.Name, path)
			responseLimit := proxyResponseLimit(repo, path)
			if res.ContentLength > responseLimit {
				utils.DiscardHTTPBody(res.Body, res.ContentLength)
				streamCancel()
				<-state.Inner.ProxyClientSemaphore
				state.Inner.InFlightDownloads.UnlockPath(pathStr, dl, false)
				return nil, fiber.ErrRequestEntityTooLarge
			}
			if res.ContentLength >= 0 && !canAllocateProxyDisk(state, uint64(res.ContentLength)) {
				utils.DiscardHTTPBody(res.Body, res.ContentLength)
				streamCancel()
				<-state.Inner.ProxyClientSemaphore
				state.Inner.InFlightDownloads.UnlockPath(pathStr, dl, false)
				return nil, fiber.NewError(fiber.StatusInsufficientStorage, "Insufficient disk space")
			}

			const maxFastPathSize = 128 * 1024
			var data []byte
			var readErr error
			switch {
			case res.ContentLength >= 0 && res.ContentLength <= maxFastPathSize:
				data = make([]byte, res.ContentLength)
				var n int
				n, readErr = io.ReadFull(res.Body, data)
				data = data[:n]
			case res.ContentLength < 0:
				data, readErr = io.ReadAll(io.LimitReader(res.Body, maxFastPathSize+1))
			}

			completeSmallResponse := readErr == nil && len(data) <= maxFastPathSize &&
				(res.ContentLength < 0 || int64(len(data)) == res.ContentLength)
			if completeSmallResponse {
				utils.DrainAndClose(res.Body)
				utils.ScheduleNetworkWorkingSetTrim()
				streamCancel()
				<-state.Inner.ProxyClientSemaphore

				saved := canAllocateProxyDisk(state, uint64(len(data))) && saveToDiskAndS3(state, repo, localFilePath, data)
				state.Inner.InFlightDownloads.UnlockPath(pathStr, dl, saved)

				return io.NopCloser(bytes.NewReader(data)), nil
			}

			var onSuccess func(string) bool
			if IsS3Enabled != nil && IsS3Enabled(repo) {
				onSuccess = func(p string) bool {
					s3Key := utils.GetS3Key(p)
					if err := UploadToS3(repo, p, s3Key); err != nil {
						return false
					}
					if err := os.Remove(p); err != nil {
						return false
					}
					notifyArtifactStored(state, repo, p)
					return true
				}
			} else if OnArtifactStored != nil || OnArtifactStoredWithState != nil {
				onSuccess = func(p string) bool {
					notifyArtifactStored(state, repo, p)
					return true
				}
			}

			var bodyReader = res.Body
			if readErr != nil {
				bodyReader = multiReadCloser{
					Reader: io.MultiReader(bytes.NewReader(data), errorReader{err: readErr}),
					Closer: res.Body,
				}
			} else if len(data) > 0 {
				bodyReader = multiReadCloser{
					Reader: io.MultiReader(bytes.NewReader(data), res.Body),
					Closer: res.Body,
				}
			}

			stream := CreateProxyStream(
				bodyReader,
				res.ContentLength,
				localFilePath,
				state.Inner.InFlightDownloads,
				pathStr,
				dl,
				state.Inner.ProxyClientSemaphore,
				streamCancel,
				state.Inner.FileIndex,
				onSuccess,
				responseLimit,
				func(next uint64) bool { return canAllocateProxyDisk(state, next) },
			)
			return stream, nil
		}

		utils.DrainAndClose(res.Body)
		streamCancel()
		<-state.Inner.ProxyClientSemaphore
	}

	if shouldNegativeCache {
		HandleNegativeCache(state, repo.Name, path, storagePath, negativeTtl)
	}
	if networkAttempted {
		utils.ScheduleNetworkWorkingSetTrim()
	}

	state.Inner.InFlightDownloads.UnlockPath(pathStr, dl, false)
	if lastBlockedReason != "" {
		return nil, fiber.NewError(fiber.StatusForbidden, lastBlockedReason)
	}
	return nil, fiber.ErrNotFound
}

// errorReader preserves a terminal error encountered while probing a response.
// Without it, a short upstream body would look like a clean EOF to the stream.
type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func ProxyHead(state *core.AppState, repo *config.Repository, path string) (bool, http.Header, error) {
	sanitizedPath, ok := utils.SanitizePath(path)
	if !ok {
		return false, nil, fiber.ErrBadRequest
	}
	path = sanitizedPath
	networkAttempted := false
	var globalProxyConfig config.ProxyConfig
	if state != nil && state.Inner != nil {
		if cfg := state.Inner.Config.Load(); cfg != nil {
			globalProxyConfig = cfg.Proxy
		}
	}
	defer func() {
		if networkAttempted {
			utils.ScheduleNetworkWorkingSetTrim()
		}
	}()

	for _, mirror := range repo.Mirrors {
		if allowed, _ := mirror.IsArtifactAllowedFor(repo.NormalizedFormat(), path); !allowed {
			continue
		}
		mirrorUrl := cargo.ArtifactURL(repo, mirror, path)
		if mirrorUrl == "" {
			continue
		}

		timeout := mirrorRequestTimeout(mirror.TimeoutSecs)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, mirrorUrl, nil)
		if err != nil {
			cancel()
			continue
		}
		if mirror.Authorization != nil {
			if err := mirror.Authorization.Apply(req); err != nil {
				cancel()
				continue
			}
		}

		select {
		case state.Inner.ProxyClientSemaphore <- struct{}{}:
		case <-ctx.Done():
			cancel()
			continue
		}

		networkAttempted = true
		client, err := clientForMirror(&mirror, globalProxyConfig)
		if err != nil {
			cancel()
			<-state.Inner.ProxyClientSemaphore
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			cancel()
			<-state.Inner.ProxyClientSemaphore
			continue
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			headers := make(http.Header, 4)
			if v := res.Header.Get("Content-Type"); v != "" {
				headers.Set("Content-Type", v)
			}
			if v := res.Header.Get("Content-Length"); v != "" {
				headers.Set("Content-Length", v)
			}
			if v := res.Header.Get("Last-Modified"); v != "" {
				headers.Set("Last-Modified", v)
			}
			if v := res.Header.Get("ETag"); v != "" {
				headers.Set("ETag", v)
			}
			utils.DrainAndClose(res.Body)
			cancel()
			<-state.Inner.ProxyClientSemaphore
			return true, headers, nil
		}
		utils.DrainAndClose(res.Body)
		cancel()
		<-state.Inner.ProxyClientSemaphore
	}

	return false, nil, nil
}

// UpstreamArtifactExists checks whether an artifact path is already present on
// any applicable mirror without caching its contents. It fails closed when an
// applicable mirror cannot return an authoritative success or not-found status.
func UpstreamArtifactExists(ctx context.Context, state *core.AppState, repo *config.Repository, path string) (bool, error) {
	if repo == nil || len(repo.Mirrors) == 0 {
		return false, nil
	}
	if state == nil || state.Inner == nil || state.Inner.ProxyClientSemaphore == nil {
		return false, ErrUpstreamProbeUnavailable
	}
	sanitizedPath, ok := utils.SanitizePath(path)
	if !ok {
		return false, fiber.ErrBadRequest
	}
	var globalProxyConfig config.ProxyConfig
	if state.Inner.Config != nil {
		if cfg := state.Inner.Config.Load(); cfg != nil {
			globalProxyConfig = cfg.Proxy
		}
	}
	var probeErr error
	for i := range repo.Mirrors {
		mirror := &repo.Mirrors[i]
		if allowed, _ := mirror.IsArtifactAllowedFor(repo.NormalizedFormat(), sanitizedPath); !allowed {
			continue
		}
		mirrorURL := cargo.ArtifactURL(repo, *mirror, sanitizedPath)
		if mirrorURL == "" {
			probeErr = errors.Join(probeErr, fmt.Errorf("mirror %d has no artifact URL", i+1))
			continue
		}

		statusCode, err := probeUpstreamArtifact(ctx, state, mirror, globalProxyConfig, mirrorURL, http.MethodHead)
		if err == nil && (statusCode == http.StatusMethodNotAllowed || statusCode == http.StatusNotImplemented) {
			statusCode, err = probeUpstreamArtifact(ctx, state, mirror, globalProxyConfig, mirrorURL, http.MethodGet)
		}
		if err != nil {
			probeErr = errors.Join(probeErr, fmt.Errorf("mirror %d: %w", i+1, err))
			continue
		}
		switch {
		case statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices:
			return true, nil
		case statusCode == http.StatusNotFound:
			continue
		default:
			probeErr = errors.Join(probeErr, fmt.Errorf("mirror %d returned status %d", i+1, statusCode))
		}
	}
	if probeErr != nil {
		return false, errors.Join(ErrUpstreamProbeUnavailable, probeErr)
	}
	return false, nil
}

func probeUpstreamArtifact(
	ctx context.Context,
	state *core.AppState,
	mirror *config.Mirror,
	proxyConfig config.ProxyConfig,
	mirrorURL, method string,
) (int, error) {
	requestCtx, cancel := context.WithTimeout(ctx, mirrorRequestTimeout(mirror.TimeoutSecs))
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, mirrorURL, nil)
	if err != nil {
		return 0, err
	}
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-0")
	}
	if mirror.Authorization != nil {
		if err := mirror.Authorization.Apply(req); err != nil {
			return 0, err
		}
	}
	client, err := clientForMirror(mirror, proxyConfig)
	if err != nil {
		return 0, err
	}
	select {
	case state.Inner.ProxyClientSemaphore <- struct{}{}:
		defer func() { <-state.Inner.ProxyClientSemaphore }()
	case <-requestCtx.Done():
		return 0, requestCtx.Err()
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	utils.DiscardHTTPBody(res.Body, res.ContentLength)
	return res.StatusCode, nil
}
