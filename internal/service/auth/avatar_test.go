/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/publicationquota"
	"renop/internal/testutil"
)

func avatarPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, value))
	return encoded.Bytes()
}

func TestNormalizeAvatarRejectsContainersAndDimensions(t *testing.T) {
	valid := avatarPNG(t, minAvatarDimension, minAvatarDimension)
	avatar, err := normalizeAvatar(valid, "image/png", int64(config.DefaultAvatarMaxSizeBytes))
	require.NoError(t, err)
	require.Equal(t, "image/png", avatar.ContentType)
	require.NotContains(t, string(avatar.Data), "PK\x03\x04")
	_, err = png.Decode(bytes.NewReader(avatar.Data))
	require.NoError(t, err)

	_, err = normalizeAvatar(append(append([]byte(nil), valid...), []byte("PK\x03\x04archive")...),
		"image/png", int64(config.DefaultAvatarMaxSizeBytes))
	require.ErrorIs(t, err, errAvatarUnsafe)
	_, err = normalizeAvatar(valid, "image/jpeg", int64(config.DefaultAvatarMaxSizeBytes))
	require.ErrorIs(t, err, errAvatarType)
	_, err = normalizeAvatar(avatarPNG(t, 256, 300), "image/png", int64(config.DefaultAvatarMaxSizeBytes))
	require.ErrorIs(t, err, errAvatarDimensions)
	_, err = normalizeAvatar(avatarPNG(t, 128, 128), "image/png", int64(config.DefaultAvatarMaxSizeBytes))
	require.ErrorIs(t, err, errAvatarDimensions)
	_, err = normalizeAvatar([]byte("GIF89a"), "image/gif", int64(config.DefaultAvatarMaxSizeBytes))
	require.ErrorIs(t, err, errAvatarType)

	jpegImage := image.NewRGBA(image.Rect(0, 0, minAvatarDimension, minAvatarDimension))
	var jpegBytes bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegBytes, jpegImage, &jpeg.Options{Quality: 85}))
	jpegAvatar, err := normalizeAvatar(jpegBytes.Bytes(), "image/jpeg", int64(config.DefaultAvatarMaxSizeBytes))
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", jpegAvatar.ContentType)
	webpBytes, err := os.ReadFile(filepath.Join("testdata", "avatar.webp"))
	require.NoError(t, err)
	webpAvatar, err := normalizeAvatar(webpBytes, "image/webp", int64(config.DefaultAvatarMaxSizeBytes))
	require.NoError(t, err)
	require.Equal(t, "image/png", webpAvatar.ContentType)
}

func TestAvatarUploadQuotaServingAndManualGitHubSync(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(testutil.TempDir(t), "avatar-routes.db"),
		MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.NoError(t, db.SaveToken(&core.AccessToken{
		Name: "alice", CreatedAt: time.Now().UTC().Format(time.RFC3339), Permissions: []string{"base"},
	}))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	require.NoError(t, db.StoreGitHubIdentity(profile.UserID, 42, "alice-gh", []core.GitHubPrincipal{{
		Type: core.GitHubPrincipalUser, GitHubID: 42, Login: "alice-gh", AuthorizedAt: time.Now().UnixMilli(),
	}}, time.Now().UnixMilli()))

	var githubRequests atomic.Int32
	githubAvatar := avatarPNG(t, 256, 256)
	githubServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		githubRequests.Add(1)
		assert.Equal(t, "/42", request.URL.Path)
		response.Header().Set("Content-Type", "image/png")
		_, _ = response.Write(githubAvatar)
	}))
	t.Cleanup(githubServer.Close)

	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(config.DefaultConfig())
	app := fiber.New(fiber.Config{StreamRequestBody: true})
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "alice", Roles: []string{"base"}})
		return c.Next()
	})
	app.Get("/users/:username/avatar", func(c fiber.Ctx) error { return getPublicAvatar(c, state) })
	auth := app.Group("/auth")
	setupAvatarRoutes(auth, state)
	setupGitHubRoutesWithProvider(auth, state, nil, githubOAuthProvider{
		AuthorizeURL: githubServer.URL, TokenURL: githubServer.URL, APIURL: githubServer.URL,
		AvatarURL: githubServer.URL,
	})

	upload := avatarPNG(t, 300, 300)
	request := httptest.NewRequest(http.MethodPut, "/auth/profile/avatar", bytes.NewReader(upload))
	request.Header.Set("Content-Type", "image/png")
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var uploadedProfile userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&uploadedProfile))
	assert.Contains(t, uploadedProfile.AvatarURL, "/api/users/alice/avatar?v=")
	assert.Equal(t, config.DefaultAvatarMaxSizeBytes, uploadedProfile.AvatarMaxSizeBytes)
	require.NoError(t, response.Body.Close())
	status, err := publicationquota.Status(state, core.PublicationQuotaSubject{
		OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: "alice",
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, status.FilesUsed)
	assert.Zero(t, status.PublicationsUsed)

	avatar, err := db.GetUserAvatar("alice")
	require.NoError(t, err)
	assert.EqualValues(t, avatar.Size, status.BytesUsed)
	getRequest := httptest.NewRequest(http.MethodGet, "/users/alice/avatar?v="+avatar.SHA256, nil)
	getResponse, err := app.Test(getRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResponse.StatusCode)
	assert.Equal(t, "image/png", getResponse.Header.Get("Content-Type"))
	assert.Equal(t, "nosniff", getResponse.Header.Get("X-Content-Type-Options"))
	assert.Contains(t, getResponse.Header.Get("Cache-Control"), "immutable")
	require.NoError(t, getResponse.Body.Close())

	unsafeRequest := httptest.NewRequest(http.MethodPut, "/auth/profile/avatar",
		bytes.NewReader(append(upload, []byte("PK\x03\x04archive")...)))
	unsafeRequest.Header.Set("Content-Type", "image/png")
	unsafeResponse, err := app.Test(unsafeRequest)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, unsafeResponse.StatusCode)
	assert.Equal(t, "avatar_unsafe", unsafeResponse.Header.Get("X-Renop-Error-Code"))
	require.NoError(t, unsafeResponse.Body.Close())

	syncResponse, err := app.Test(httptest.NewRequest(http.MethodPost, "/auth/profile/avatar/github", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, syncResponse.StatusCode)
	require.NoError(t, syncResponse.Body.Close())
	assert.EqualValues(t, 1, githubRequests.Load())
	status, err = publicationquota.Status(state, core.PublicationQuotaSubject{
		OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: "alice",
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, status.FilesUsed)
	assert.Zero(t, status.PublicationsUsed)

	deleteResponse, err := app.Test(httptest.NewRequest(http.MethodDelete, "/auth/profile/avatar", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, deleteResponse.StatusCode)
	require.NoError(t, deleteResponse.Body.Close())
	_, err = db.GetUserAvatar("alice")
	require.ErrorIs(t, err, core.ErrUserAvatarNotFound)

	request = httptest.NewRequest(http.MethodPut, "/auth/profile/avatar", strings.NewReader("not an image"))
	request.Header.Set("Content-Type", "image/png")
	response, err = app.Test(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
}
