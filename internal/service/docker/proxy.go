/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/proxy"
	"renop/internal/utils"
)

var (
	upstreamTokenCache pbMapOfTokens
)

// ErrUpstreamImageProbeUnavailable indicates that at least one applicable
// Docker mirror could not provide an authoritative image-name result.
var ErrUpstreamImageProbeUnavailable = errors.New("upstream Docker image availability check failed")

type pbMapOfTokens struct {
	sync.Map
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// UpstreamImageExists checks whether an image name is already occupied by any
// applicable mirror without importing its manifest into the local catalog.
func UpstreamImageExists(ctx context.Context, state *core.AppState, repo *config.Repository, imageName string) (bool, error) {
	if repo == nil || len(repo.Mirrors) == 0 {
		return false, nil
	}
	var probeErr error
	for i := range repo.Mirrors {
		mirror := repo.Mirrors[i]
		if allowed, _ := mirror.IsArtifactAllowedFor(config.RepositoryFormatDocker, imageName); !allowed {
			continue
		}
		statusCode, err := probeMirrorImageSingle(ctx, state, mirror, imageName)
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
		return false, errors.Join(ErrUpstreamImageProbeUnavailable, probeErr)
	}
	return false, nil
}

func probeMirrorImageSingle(
	ctx context.Context,
	state *core.AppState,
	mirror config.Mirror,
	imageName string,
) (int, error) {
	base := strings.TrimRight(strings.TrimSpace(mirror.URL), "/")
	if base == "" {
		return 0, errors.New("empty mirror URL")
	}
	upstreamImage := imageName
	if strings.Contains(base, "docker.io") {
		upstreamImage = normalizeUpstreamImage(imageName)
	}
	tagsURL := fmt.Sprintf("%s/v2/%s/tags/list?n=1", base, upstreamImage)
	var proxyCfg config.ProxyConfig
	if state != nil && state.Inner != nil && state.Inner.Config != nil {
		if cfg := state.Inner.Config.Load(); cfg != nil {
			proxyCfg = cfg.Proxy
		}
	}
	client, err := proxy.ClientForMirror(&mirror, proxyCfg)
	if err != nil {
		return 0, err
	}
	if client == nil {
		return 0, errors.New("docker mirror client is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if err := applyDockerMirrorAuth(ctx, client, req, mirror, upstreamImage, "pull"); err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	utils.DiscardHTTPBody(resp.Body, resp.ContentLength)
	return resp.StatusCode, nil
}

// FetchUpstreamManifest fetches a manifest from upstream mirrors with authentication and caching.
func FetchUpstreamManifest(
	ctx context.Context,
	state *core.AppState,
	repo *config.Repository,
	imageName, reference string,
) ([]byte, string, string, error) {
	if repo == nil || len(repo.Mirrors) == 0 {
		return nil, "", "", errors.New("no mirrors configured")
	}

	var validationErr error
	for _, mirror := range repo.Mirrors {
		if allowed, _ := mirror.IsArtifactAllowedFor(config.RepositoryFormatDocker, imageName); !allowed {
			continue
		}

		data, mediaType, digest, err := fetchMirrorManifestSingle(ctx, state, mirror, imageName, reference)
		if err == nil && len(data) > 0 {
			return data, mediaType, digest, nil
		}
		if errors.Is(err, ErrManifestTooLarge) || errors.Is(err, ErrManifestDigestMismatch) {
			validationErr = err
		}
	}
	if validationErr != nil {
		return nil, "", "", validationErr
	}

	return nil, "", "", errors.New("manifest not found on any mirror")
}

func normalizeUpstreamImage(imageName string) string {
	imageName = strings.Trim(imageName, "/")
	if !strings.Contains(imageName, "/") {
		return "library/" + imageName
	}
	return imageName
}

func fetchMirrorManifestSingle(
	ctx context.Context,
	state *core.AppState,
	mirror config.Mirror,
	imageName, reference string,
) ([]byte, string, string, error) {
	client, req, err := newMirrorManifestRequest(ctx, state, mirror, imageName, reference, http.MethodGet)
	if err != nil {
		return nil, "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxManifestSize {
		return nil, "", "", ErrManifestTooLarge
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxManifestSize+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(body) > MaxManifestSize {
		return nil, "", "", ErrManifestTooLarge
	}

	mediaType := resp.Header.Get("Content-Type")
	digest := resp.Header.Get(DockerDigestHeader)
	calculatedDigest := CalculateDigest(body)
	if digest == "" {
		digest = calculatedDigest
	} else if digest != calculatedDigest {
		return nil, "", "", ErrManifestDigestMismatch
	}

	return body, mediaType, digest, nil
}

func newMirrorManifestRequest(
	ctx context.Context,
	state *core.AppState,
	mirror config.Mirror,
	imageName, reference, method string,
) (*http.Client, *http.Request, error) {
	base := strings.TrimRight(strings.TrimSpace(mirror.URL), "/")
	if base == "" {
		return nil, nil, errors.New("empty mirror URL")
	}

	upstreamImage := imageName
	if strings.Contains(base, "docker.io") {
		upstreamImage = normalizeUpstreamImage(imageName)
	}

	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", base, upstreamImage, url.PathEscape(reference))
	var proxyCfg config.ProxyConfig
	if state != nil && state.Inner != nil && state.Inner.Config != nil {
		if c := state.Inner.Config.Load(); c != nil {
			proxyCfg = c.Proxy
		}
	}
	client, err := proxy.ClientForMirror(&mirror, proxyCfg)
	if err != nil || client == nil {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("docker mirror client is unavailable")
	}

	req, err := http.NewRequestWithContext(ctx, method, manifestURL, nil)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Accept", strings.Join([]string{
		MediaTypeDockerManifest2,
		MediaTypeOCIManifest1,
		MediaTypeDockerManifestList,
		MediaTypeOCIIndex1,
	}, ", "))

	// Apply configured mirror auth or obtain upstream token
	if err := applyDockerMirrorAuth(ctx, client, req, mirror, upstreamImage, "pull"); err != nil {
		return nil, nil, err
	}
	return client, req, nil
}

// FetchUpstreamBlob streams a blob from configured upstream mirrors.
func FetchUpstreamBlob(
	ctx context.Context,
	state *core.AppState,
	repo *config.Repository,
	imageName, digest string,
) (io.ReadCloser, int64, error) {
	if repo == nil || len(repo.Mirrors) == 0 {
		return nil, 0, errors.New("no mirrors configured")
	}

	for _, mirror := range repo.Mirrors {
		if allowed, _ := mirror.IsArtifactAllowedFor(config.RepositoryFormatDocker, imageName); !allowed {
			continue
		}

		rc, size, err := fetchMirrorBlobSingle(ctx, state, mirror, imageName, digest)
		if err == nil && rc != nil {
			return rc, size, nil
		}
	}

	return nil, 0, errors.New("blob not found on any mirror")
}

func fetchMirrorBlobSingle(
	ctx context.Context,
	state *core.AppState,
	mirror config.Mirror,
	imageName, digest string,
) (io.ReadCloser, int64, error) {
	base := strings.TrimRight(strings.TrimSpace(mirror.URL), "/")
	if base == "" {
		return nil, 0, errors.New("empty mirror URL")
	}

	upstreamImage := imageName
	if strings.Contains(base, "docker.io") {
		upstreamImage = normalizeUpstreamImage(imageName)
	}

	blobURL := fmt.Sprintf("%s/v2/%s/blobs/%s", base, upstreamImage, digest)
	var proxyCfg config.ProxyConfig
	if state != nil && state.Inner != nil && state.Inner.Config != nil {
		if c := state.Inner.Config.Load(); c != nil {
			proxyCfg = c.Proxy
		}
	}
	client, err := proxy.ClientForMirror(&mirror, proxyCfg)
	if err != nil || client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL, nil)
	if err != nil {
		return nil, 0, err
	}

	if err := applyDockerMirrorAuth(ctx, client, req, mirror, upstreamImage, "pull"); err != nil {
		return nil, 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf("upstream blob returned status %d", resp.StatusCode)
	}

	return resp.Body, resp.ContentLength, nil
}

func applyDockerMirrorAuth(
	ctx context.Context,
	client *http.Client,
	req *http.Request,
	mirror config.Mirror,
	imageName, action string,
) error {
	if mirror.Authorization != nil && strings.ToLower(mirror.Authorization.Method) == "bearer" {
		return mirror.Authorization.Apply(req)
	}

	cacheKey := fmt.Sprintf("%s|%s|%s", mirror.URL, imageName, action)
	if val, ok := upstreamTokenCache.Load(cacheKey); ok {
		tok := val.(cachedToken)
		if time.Now().Before(tok.expiresAt) {
			req.Header.Set("Authorization", "Bearer "+tok.token)
			return nil
		}
	}

	// Probe upstream for auth challenge
	probeURL := fmt.Sprintf("%s/v2/", strings.TrimRight(mirror.URL, "/"))
	probeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return err
	}
	if mirror.Authorization != nil {
		_ = mirror.Authorization.Apply(probeReq)
	}

	probeResp, err := client.Do(probeReq)
	if err != nil {
		return err
	}
	_ = probeResp.Body.Close()

	if probeResp.StatusCode == http.StatusOK {
		if mirror.Authorization != nil {
			return mirror.Authorization.Apply(req)
		}
		return nil
	}

	authHeader := probeResp.Header.Get("Www-Authenticate")
	if authHeader == "" {
		if mirror.Authorization != nil {
			return mirror.Authorization.Apply(req)
		}
		return nil
	}

	token, ttl, err := exchangeUpstreamToken(ctx, client, authHeader, mirror, imageName, action)
	if err != nil {
		// Fallback to basic credentials if token exchange fails
		if mirror.Authorization != nil {
			return mirror.Authorization.Apply(req)
		}
		return err
	}

	if token != "" {
		upstreamTokenCache.Store(cacheKey, cachedToken{
			token:     token,
			expiresAt: time.Now().Add(time.Duration(ttl-30) * time.Second),
		})
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return nil
}

func exchangeUpstreamToken(
	ctx context.Context,
	client *http.Client,
	wwwAuth string,
	mirror config.Mirror,
	imageName, action string,
) (string, int, error) {
	if !strings.HasPrefix(wwwAuth, "Bearer ") {
		return "", 0, errors.New("unsupported challenge type")
	}

	challenge := strings.TrimPrefix(wwwAuth, "Bearer ")
	params := parseChallengeParams(challenge)
	realm := params["realm"]
	service := params["service"]
	if realm == "" {
		return "", 0, errors.New("missing realm in challenge")
	}

	u, err := url.Parse(realm)
	if err != nil {
		return "", 0, err
	}

	q := u.Query()
	if service != "" {
		q.Set("service", service)
	}
	q.Set("scope", fmt.Sprintf("repository:%s:%s", imageName, action))
	u.RawQuery = q.Encode()

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", 0, err
	}

	if mirror.Authorization != nil && (mirror.Authorization.Method == "basic" || mirror.Authorization.Method == "username/password") {
		tokenReq.SetBasicAuth(mirror.Authorization.Login, mirror.Authorization.Password)
	}

	resp, err := client.Do(tokenReq)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}

	tok := result.Token
	if tok == "" {
		tok = result.AccessToken
	}
	ttl := result.ExpiresIn
	if ttl <= 0 {
		ttl = 300
	}

	return tok, ttl, nil
}

func parseChallengeParams(challenge string) map[string]string {
	params := make(map[string]string)
	parts := strings.SplitSeq(challenge, ",")
	for part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.Trim(strings.TrimSpace(kv[1]), `"`)
			params[k] = v
		}
	}
	return params
}

// CopyAndHash streams src to dst while computing sha256.
func CopyAndHash(dst io.Writer, src io.Reader) (string, int64, error) {
	hasher := sha256.New()
	mw := io.MultiWriter(dst, hasher)
	n, err := io.Copy(mw, src)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), n, nil
}
