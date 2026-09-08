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
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"
	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/testutil"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func TestMFASetupLoginReplayAndPolicyBoundaries(t *testing.T) {
	db := newTestAuthDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	cfg := config.DefaultConfig()
	state.Inner.Config.Store(cfg)
	keyPath := filepath.Join(testutil.TempDir(t), "config.yaml")
	require.NoError(t, EnsureMFAKey(state, keyPath))
	key := state.Inner.Config.Load().MFAEncryptionKey
	require.NoError(t, EnsureMFAKey(state, keyPath))
	require.Equal(t, key, state.Inner.Config.Load().MFAEncryptionKey)
	publicConfig, err := json.Marshal(state.Inner.Config.Load())
	require.NoError(t, err)
	require.NotContains(t, string(publicConfig), key)
	password, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	require.NoError(t, err)
	account := &core.AccessToken{Name: "alice", EncryptedSecret: string(password), Permissions: []string{"base"}, Tokens: []string{"legacy-upload-token"}}
	require.NoError(t, db.SaveToken(account))
	state.Inner.TokensCount.Store(1)
	app := fiber.New()
	app.Use(AuthMiddleware(state))
	SetupAuthRoutes(app.Group("/api"), state, nil)
	request := func(method, path string, body any, cookies ...*http.Cookie) *http.Response {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		contentType := "application/json"
		if path == "/api/auth/login" {
			data, err = proto.Marshal(&pb.LoginRequest{Name: "alice", Secret: "test-password"})
			require.NoError(t, err)
			contentType = protohttp.ContentType
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Content-Type", contentType)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	cookieNamed := func(response *http.Response, name string) *http.Cookie {
		for _, cookie := range response.Cookies() {
			if cookie.Name == name {
				return cookie
			}
		}
		t.Fatalf("missing cookie %s for status %d", name, response.StatusCode)
		return nil
	}
	login := func() *http.Response {
		return request("POST", "/api/auth/login", map[string]string{"name": "alice", "secret": "test-password"})
	}
	initial := login()
	require.Equal(t, 200, initial.StatusCode)
	session := cookieNamed(initial, sessionCookieName)
	other := login()
	otherSession := cookieNamed(other, sessionCookieName)
	begin := request("POST", "/api/auth/profile/mfa/totp/begin", map[string]any{}, session)
	require.Equal(t, 200, begin.StatusCode)
	var setup struct{ ID, Secret, URI, QR string }
	require.NoError(t, json.NewDecoder(begin.Body).Decode(&setup))
	require.Len(t, setup.Secret, 32)
	require.Contains(t, setup.URI, "otpauth://totp/")
	require.Contains(t, setup.URI, "digits=6")
	png, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(setup.QR, "data:image/png;base64,"))
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(png, []byte("\x89PNG\r\n\x1a\n")))
	wrong := request("POST", "/api/auth/profile/mfa/totp/confirm", map[string]string{"id": setup.ID, "code": "bad"}, session)
	require.Equal(t, 400, wrong.StatusCode)
	step := time.Now().Unix() / 30
	confirmed := request("POST", "/api/auth/profile/mfa/totp/confirm", map[string]string{"id": setup.ID, "code": core.TOTPCode(setup.Secret, step-1)}, session)
	require.Equal(t, 200, confirmed.StatusCode)
	body, err := io.ReadAll(confirmed.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), setup.Secret)
	require.Contains(t, string(body), `"totp_enabled":true`)
	mfa, err := db.GetMFAState("alice")
	require.NoError(t, err)
	require.NotContains(t, mfa.Secret, setup.Secret)
	plain, err := decryptMFASecret(state, mfa)
	require.NoError(t, err)
	require.Equal(t, setup.Secret, plain)
	swapped := *mfa
	swapped.UserID = "another-user"
	_, err = decryptMFASecret(state, &swapped)
	require.Error(t, err)
	require.Equal(t, 401, request("GET", "/api/auth/me", nil, otherSession).StatusCode)
	require.Equal(t, 200, request("GET", "/api/auth/me", nil, session).StatusCode)
	credential, err := VerifyAccountCredential(state, account, "test-password")
	require.NoError(t, err)
	require.Nil(t, credential)
	credential, err = VerifyAccountCredential(state, account, "legacy-upload-token")
	require.NoError(t, err)
	require.NotNil(t, credential)
	challengeResponse := login()
	require.Equal(t, 409, challengeResponse.StatusCode)
	require.Equal(t, "MFA_REQUIRED", challengeResponse.Header.Get("X-Renop-Error-Code"))
	challengeCookie := cookieNamed(challengeResponse, mfaCookieName)
	require.True(t, challengeCookie.HttpOnly)
	for _, cookie := range challengeResponse.Cookies() {
		require.NotEqual(t, sessionCookieName, cookie.Name)
	}
	require.Equal(t, 401, request("GET", "/api/auth/me", nil, challengeCookie).StatusCode)
	require.Equal(t, 200, request("GET", "/api/auth/mfa", nil, challengeCookie).StatusCode)
	verified := request("POST", "/api/auth/mfa/totp", map[string]string{"code": core.TOTPCode(setup.Secret, step)}, challengeCookie)
	require.Equal(t, 200, verified.StatusCode)
	verifiedCookie := cookieNamed(verified, sessionCookieName)
	require.Equal(t, 200, request("GET", "/api/auth/me", nil, verifiedCookie).StatusCode)
	require.Equal(t, 400, request("POST", "/api/auth/mfa/totp", map[string]string{"code": core.TOTPCode(setup.Secret, step)}, challengeCookie).StatusCode)
	secondChallenge := cookieNamed(login(), mfaCookieName)
	require.Equal(t, 400, request("POST", "/api/auth/mfa/totp", map[string]string{"code": core.TOTPCode(setup.Secret, step)}, secondChallenge).StatusCode)
	for range 4 {
		require.Equal(t, 400, request("POST", "/api/auth/mfa/totp", map[string]string{"code": "invalid"}, secondChallenge).StatusCode)
	}
	freshChallenge := cookieNamed(login(), mfaCookieName)
	require.Equal(t, 400, request("POST", "/api/auth/mfa/totp", map[string]string{"code": core.TOTPCode(setup.Secret, step+1)}, freshChallenge).StatusCode)
	removed := request("PUT", "/api/auth/profile/mfa", map[string]bool{"totp_enabled": false}, verifiedCookie)
	require.Equal(t, 200, removed.StatusCode)
	require.Equal(t, 400, request("POST", "/api/auth/mfa/totp", map[string]string{"code": core.TOTPCode(setup.Secret, step+1)}, freshChallenge).StatusCode)
	require.Equal(t, 200, login().StatusCode)
	oldSession, err := db.GetSession(verifiedCookie.Value)
	require.NoError(t, err)
	require.NotNil(t, oldSession)
	oldSession.CreatedAt = time.Now().Add(-6 * time.Minute).UnixMilli()
	require.NoError(t, db.SaveSession(oldSession, verifiedCookie.Value))
	denied := request("POST", "/api/auth/profile/mfa/totp/begin", map[string]any{}, verifiedCookie)
	require.Equal(t, 403, denied.StatusCode)
	require.Equal(t, "MFA_REAUTH_REQUIRED", denied.Header.Get("X-Renop-Error-Code"))
	require.Equal(t, 401, request("POST", "/api/auth/profile/mfa/totp/begin", map[string]any{}).StatusCode)
}

func TestMFACounterIsAtomicAndRejectsStaleSessions(t *testing.T) {
	db := newTestAuthDB(t)
	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "alice", EncryptedSecret: "hash", Permissions: []string{"base"}}))
	session := &core.Session{Username: "alice", PublicID: "current", CreatedAt: time.Now().UnixMilli()}
	session.LastActive.Store(session.CreatedAt)
	require.NoError(t, db.SaveSession(session, "current"))
	mfa, err := db.GetMFAState("alice")
	require.NoError(t, err)
	before := mfa.Snapshot
	require.NoError(t, db.UpdateMFA("alice", mfa.Snapshot, "ciphertext", false, 0, "current"))
	session.AuthenticationSnapshot = before
	require.ErrorIs(t, db.SaveSession(session, "stale"), core.ErrMFAInvalid)
	mfa, err = db.GetMFAState("alice")
	require.NoError(t, err)
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Go(func() { results <- db.ConsumeMFACode("alice", mfa.Revision, 100, time.Now().UnixMilli()) })
	}
	group.Wait()
	close(results)
	passed := 0
	for err := range results {
		if err == nil {
			passed++
		} else {
			require.ErrorIs(t, err, core.ErrMFAInvalid)
		}
	}
	require.Equal(t, 1, passed)
}
