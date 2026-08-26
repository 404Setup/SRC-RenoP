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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/core"
)

func TestDockerTokenLifecycle(t *testing.T) {
	secret := []byte("a-very-secure-docker-secret-key-32bytes")
	access := []AccessEntry{
		{
			Type:    "repository",
			Name:    "docker-local/org/my-app",
			Actions: []string{"pull", "push"},
		},
	}

	token, err := GenerateDockerToken(secret, "admin", "renop-registry", access, 10*time.Minute)
	if err != nil {
		t.Fatalf("GenerateDockerToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := ValidateDockerToken(secret, token)
	if err != nil {
		t.Fatalf("ValidateDockerToken failed: %v", err)
	}
	if claims.Subject != "admin" {
		t.Fatalf("expected subject 'admin', got '%s'", claims.Subject)
	}
	if claims.Audience != "renop-registry" {
		t.Fatalf("expected audience 'renop-registry', got '%s'", claims.Audience)
	}
	if len(claims.Access) != 1 || claims.Access[0].Name != "docker-local/org/my-app" {
		t.Fatalf("unexpected access entries: %+v", claims.Access)
	}
	if len(claims.Access[0].Actions) != 2 || claims.Access[0].Actions[0] != "pull" || claims.Access[0].Actions[1] != "push" {
		t.Fatalf("unexpected actions: %+v", claims.Access[0].Actions)
	}

	expiredToken, err := GenerateDockerToken(secret, "admin", "renop-registry", access, -5*time.Minute)
	if err != nil {
		t.Fatalf("GenerateDockerToken (expired) failed: %v", err)
	}
	_, err = ValidateDockerToken(secret, expiredToken)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected token expired error, got: %v", err)
	}

	tamperedToken := token + "tamper"
	_, err = ValidateDockerToken(secret, tamperedToken)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected invalid signature error, got: %v", err)
	}

	wrongSecret := []byte("wrong-secret-key-000000000000000000")
	_, err = ValidateDockerToken(wrongSecret, token)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature mismatch with wrong secret, got: %v", err)
	}

	malformedCases := []string{
		"",
		"singlepart",
		"part1.part2",
		"part1.part2.part3.part4",
	}
	for _, mal := range malformedCases {
		if _, err := ValidateDockerToken(secret, mal); err == nil {
			t.Fatalf("expected error for malformed token '%s'", mal)
		}
	}

	parts := strings.Split(token, ".")
	fakeClaimsJSON, _ := json.Marshal(TokenClaims{
		Issuer:    "renop",
		Subject:   "root-attacker",
		Audience:  "renop-registry",
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	})
	fakeClaimsEncoded := base64.RawURLEncoding.EncodeToString(fakeClaimsJSON)
	forgedToken := parts[0] + "." + fakeClaimsEncoded + "." + parts[2]
	if _, err := ValidateDockerToken(secret, forgedToken); err == nil {
		t.Fatal("expected signature mismatch for forged payload token")
	}

	msg := parts[0] + ".???invalidbase64???"
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, err := ValidateDockerToken(secret, msg+"."+sig); err == nil {
		t.Fatal("expected decode error for corrupted claims encoding")
	}
}

func TestDockerBasicAuthHonorsPasswordLoginPolicy(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "policy-app", false)
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("account-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	now := time.Now().UnixMilli()
	require.NoError(t, state.GetDB().SetAccountPassword("admin", string(passwordHash), now))
	require.NoError(t, state.GetDB().SaveFidoDevice(&core.FidoDevice{
		ID: "admin-key", Username: "admin", Name: "Passkey", CredentialID: []byte("admin-credential"),
		PublicKey: []byte("public"), CreatedAt: now,
	}))
	_, err = state.GetDB().SetPasswordLoginEnabled("admin", false, now+1)
	require.NoError(t, err)

	const tokenURL = "/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/policy-app:pull,push"
	passwordRequest := httptest.NewRequest(http.MethodGet, tokenURL, nil)
	passwordRequest.SetBasicAuth("admin", "account-password")
	passwordResponse, err := app.Test(passwordRequest)
	require.NoError(t, err)
	var passwordToken TokenResponse
	require.NoError(t, json.NewDecoder(passwordResponse.Body).Decode(&passwordToken))
	require.NoError(t, passwordResponse.Body.Close())
	require.Equal(t, http.StatusOK, passwordResponse.StatusCode)
	passwordClaims, err := ValidateDockerToken(state.GetDockerSecret(), passwordToken.Token)
	require.NoError(t, err)
	require.Equal(t, "guest", passwordClaims.Subject)
	require.Len(t, passwordClaims.Access, 1)
	require.NotContains(t, passwordClaims.Access[0].Actions, "push")

	tokenRequest := httptest.NewRequest(http.MethodGet, tokenURL, nil)
	tokenRequest.SetBasicAuth("admin", "admin-secret-token")
	tokenResponse, err := app.Test(tokenRequest)
	require.NoError(t, err)
	var apiToken TokenResponse
	require.NoError(t, json.NewDecoder(tokenResponse.Body).Decode(&apiToken))
	require.NoError(t, tokenResponse.Body.Close())
	require.Equal(t, http.StatusOK, tokenResponse.StatusCode)
	apiClaims, err := ValidateDockerToken(state.GetDockerSecret(), apiToken.Token)
	require.NoError(t, err)
	require.Equal(t, "admin", apiClaims.Subject)
	require.Len(t, apiClaims.Access, 1)
	require.ElementsMatch(t, []string{"pull", "push", "delete"}, apiClaims.Access[0].Actions)
}

func TestDockerTokenActionsRespectAPITokenScopes(t *testing.T) {
	app, state, _ := setupTestDockerApp(t)
	createTestDockerImage(t, state, "docker-local", "scoped-app", false)
	secret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	require.NoError(t, state.GetDB().CreateAPIToken("admin", &core.APIToken{
		ID: uuid.NewString(), Name: "Pull only", Scopes: []string{core.APITokenScopeRepositoryRead},
		CreatedAt: time.Now().UnixMilli(),
	}, core.HashAPITokenSecret(secret)))

	request := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/scoped-app:pull,push", nil)
	request.SetBasicAuth("admin", secret)
	response, err := app.Test(request)
	require.NoError(t, err)
	var tokenResponse TokenResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&tokenResponse))
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	claims, err := ValidateDockerToken(state.GetDockerSecret(), tokenResponse.Token)
	require.NoError(t, err)
	require.Len(t, claims.Access, 1)
	require.Equal(t, []string{"pull"}, claims.Access[0].Actions)

	restrictedSecret, err := core.GenerateAPITokenSecret()
	require.NoError(t, err)
	require.NoError(t, state.GetDB().CreateAPIToken("admin", &core.APIToken{
		ID: uuid.NewString(), Name: "Other repository only", Scopes: []string{core.APITokenScopeRepositoryRead},
		Targets:   map[string][]string{core.APITokenScopeRepositoryRead: {"other"}},
		CreatedAt: time.Now().UnixMilli(),
	}, core.HashAPITokenSecret(restrictedSecret)))
	restrictedRequest := httptest.NewRequest(http.MethodGet,
		"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/scoped-app:pull", nil)
	restrictedRequest.SetBasicAuth("admin", restrictedSecret)
	restrictedResponse, err := app.Test(restrictedRequest)
	require.NoError(t, err)
	var restrictedToken TokenResponse
	require.NoError(t, json.NewDecoder(restrictedResponse.Body).Decode(&restrictedToken))
	require.NoError(t, restrictedResponse.Body.Close())
	restrictedClaims, err := ValidateDockerToken(state.GetDockerSecret(), restrictedToken.Token)
	require.NoError(t, err)
	require.Len(t, restrictedClaims.Access, 1)
	require.Empty(t, restrictedClaims.Access[0].Actions)

	for _, test := range []struct {
		name    string
		scope   string
		actions []string
	}{
		{name: "Publish only", scope: core.APITokenScopeRepositoryPublish, actions: []string{"push"}},
		{name: "Delete only", scope: core.APITokenScopeRepositoryDelete, actions: []string{"delete"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tokenSecret, generateErr := core.GenerateAPITokenSecret()
			require.NoError(t, generateErr)
			require.NoError(t, state.GetDB().CreateAPIToken("admin", &core.APIToken{
				ID: uuid.NewString(), Name: test.name, Scopes: []string{test.scope},
				CreatedAt: time.Now().UnixMilli(),
			}, core.HashAPITokenSecret(tokenSecret)))
			tokenRequest := httptest.NewRequest(http.MethodGet,
				"/v2/token?service=127.0.0.1:8080&scope=repository:docker-local/scoped-app:pull,push", nil)
			tokenRequest.SetBasicAuth("admin", tokenSecret)
			tokenResponseRaw, requestErr := app.Test(tokenRequest)
			require.NoError(t, requestErr)
			var issued TokenResponse
			require.NoError(t, json.NewDecoder(tokenResponseRaw.Body).Decode(&issued))
			require.NoError(t, tokenResponseRaw.Body.Close())
			issuedClaims, validateErr := ValidateDockerToken(state.GetDockerSecret(), issued.Token)
			require.NoError(t, validateErr)
			require.Len(t, issuedClaims.Access, 1)
			require.Equal(t, test.actions, issuedClaims.Access[0].Actions)
			if test.scope == core.APITokenScopeRepositoryPublish {
				deleteRequest := httptest.NewRequest(http.MethodDelete,
					"/v2/docker-local/scoped-app/manifests/missing", nil)
				deleteRequest.SetBasicAuth("admin", tokenSecret)
				deleteResponse, deleteErr := app.Test(deleteRequest)
				require.NoError(t, deleteErr)
				require.NoError(t, deleteResponse.Body.Close())
				require.Equal(t, http.StatusForbidden, deleteResponse.StatusCode)
			}
		})
	}
}
