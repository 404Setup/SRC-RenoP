/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package gpg

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func TestAddProfileKeyUsesCachedFingerprint(t *testing.T) {
	state, db := testGPGState(t)
	_, key, aliases := testSigningEntity(t)
	require.NoError(t, db.RefreshGPGPublicKey(key, aliases))

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: "alice"})
		return c.Next()
	})
	SetupProfileRoutes(app.Group("/api"), state)

	body, err := proto.Marshal(&pb.GpgKeyReferenceRequest{KeyId: key.Fingerprint})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/profile/gpg", bytes.NewReader(body))
	request.Header.Set("Content-Type", protohttp.ContentType)
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusCreated, response.StatusCode)

	registered, err := db.ListUserGPGKeys("alice")
	require.NoError(t, err)
	require.Len(t, registered, 1)
	assert.Equal(t, key.Fingerprint, registered[0].Fingerprint)
}

func TestProfileReleaseHistoryIsScopedToAuthenticatedUser(t *testing.T) {
	state, db := testGPGState(t)
	now := time.Now().UnixMilli()
	for i, username := range []string{"alice", "bob"} {
		require.NoError(t, db.SaveGPGRelease(&core.GPGRelease{
			ID:             fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1),
			Repository:     "releases",
			ArtifactPath:   fmt.Sprintf("org/example/demo-%d.jar", i+1),
			Uploader:       username,
			Status:         core.GPGReleaseSuccess,
			CreatedAt:      now + int64(i),
			UpdatedAt:      now + int64(i),
			CompletedAt:    now + int64(i),
			CleanupPending: false,
		}))
	}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		if username := c.Get("X-Test-User"); username != "" {
			c.Locals("user", &config.User{Username: username})
		}
		return c.Next()
	})
	SetupProfileRoutes(app.Group("/api"), state)

	unauthorized, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/auth/profile/gpg/releases", nil))
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, unauthorized.StatusCode)
	require.NoError(t, unauthorized.Body.Close())

	request := httptest.NewRequest(http.MethodGet, "/api/auth/profile/gpg/releases?limit=500&offset=-3", nil)
	request.Header.Set("X-Test-User", "Alice")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	var history pb.GpgReleaseList
	require.NoError(t, proto.Unmarshal(body, &history))
	assert.Equal(t, int32(1), history.Total)
	assert.Equal(t, int32(100), history.Limit)
	assert.Equal(t, int32(0), history.Offset)
	require.Len(t, history.Releases, 1)
	assert.Equal(t, "org/example/demo-1.jar", history.Releases[0].ArtifactPath)
}
