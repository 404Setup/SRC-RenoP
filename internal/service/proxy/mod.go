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
	"renop/internal/service/index"
	"renop/internal/service/status"
	"renop/internal/utils"
)

var (
	IsS3Enabled      func(repo *config.Repository) bool
	UploadToS3       func(repo *config.Repository, localPath string, s3Key string) error
	UploadStreamToS3 func(repo *config.Repository, s3Key string, reader io.Reader, size int64, contentType string) error
	OnArtifactStored func(localPath string)
)

var httpClient = newProxyHTTPClient()

func newProxyHTTPClient() *http.Client {
	client := utils.OutboundClient(0)
	client.CheckRedirect = checkMirrorRedirect
	return client
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

func escapeArtifactPath(path string) string {
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(parts[i]), "%2B", "+")
	}
	return strings.Join(parts, "/")
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
			if OnArtifactStored != nil {
				OnArtifactStored(localFilePath)
			}
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
		if OnArtifactStored != nil {
			OnArtifactStored(localFilePath)
		}

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
	encodedPath := escapeArtifactPath(path)

	var lastBlockedReason string
	for _, mirror := range repo.Mirrors {
		if allowed, reason := mirror.IsArtifactAllowed(path); !allowed {
			lastBlockedReason = reason
			continue
		}
		trimmedUrl := strings.TrimRight(mirror.Url, "/")

		var builder strings.Builder
		builder.Grow(len(trimmedUrl) + 1 + len(encodedPath))
		builder.WriteString(trimmedUrl)
		builder.WriteByte('/')
		builder.WriteString(encodedPath)
		mirrorUrl := builder.String()

		streamCtx, streamCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, mirrorUrl, nil)
		if err != nil {
			streamCancel()
			continue
		}
		if mirror.Authorization != nil {
			if header := mirror.Authorization.GetAuthHeader(); header != "" {
				req.Header.Set("Authorization", header)
			}
		}

		select {
		case state.Inner.ProxyClientSemaphore <- struct{}{}:
		case <-streamCtx.Done():
			streamCancel()
			continue
		}

		networkAttempted = true
		res, err := httpClient.Do(req)
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

				saved := saveToDiskAndS3(state, repo, localFilePath, data)
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
					if OnArtifactStored != nil {
						OnArtifactStored(p)
					}
					return true
				}
			} else if OnArtifactStored != nil {
				onSuccess = func(p string) bool {
					OnArtifactStored(p)
					return true
				}
			}

			var bodyReader io.ReadCloser = res.Body
			if readErr != nil {
				bodyReader = struct {
					io.Reader
					io.Closer
				}{
					Reader: io.MultiReader(bytes.NewReader(data), errorReader{err: readErr}),
					Closer: res.Body,
				}
			} else if len(data) > 0 {
				bodyReader = struct {
					io.Reader
					io.Closer
				}{
					Reader: io.MultiReader(bytes.NewReader(data), res.Body),
					Closer: res.Body,
				}
			}

			stream := CreateProxyStream(bodyReader, res.ContentLength, localFilePath, state.Inner.InFlightDownloads, pathStr, dl, state.Inner.ProxyClientSemaphore, streamCancel, state.Inner.FileIndex, onSuccess)
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
	encodedPath := escapeArtifactPath(path)
	networkAttempted := false
	defer func() {
		if networkAttempted {
			utils.ScheduleNetworkWorkingSetTrim()
		}
	}()

	for _, mirror := range repo.Mirrors {
		if allowed, _ := mirror.IsArtifactAllowed(path); !allowed {
			continue
		}
		trimmedUrl := strings.TrimRight(mirror.Url, "/")
		mirrorUrl := trimmedUrl + "/" + encodedPath

		timeout := time.Duration(mirror.TimeoutSecs) * time.Second
		if timeout == 0 {
			timeout = 5 * time.Second
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, mirrorUrl, nil)
		if err != nil {
			cancel()
			continue
		}
		if mirror.Authorization != nil {
			if header := mirror.Authorization.GetAuthHeader(); header != "" {
				req.Header.Set("Authorization", header)
			}
		}

		select {
		case state.Inner.ProxyClientSemaphore <- struct{}{}:
		case <-ctx.Done():
			cancel()
			continue
		}

		networkAttempted = true
		res, err := httpClient.Do(req)
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
