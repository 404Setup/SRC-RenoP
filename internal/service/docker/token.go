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
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

const dockerTokenHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"

type AccessEntry struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

type TokenClaims struct {
	Issuer    string        `json:"iss"`
	Subject   string        `json:"sub"`
	Audience  string        `json:"aud"`
	ExpiresAt int64         `json:"exp"`
	NotBefore int64         `json:"nbf"`
	IssuedAt  int64         `json:"iat"`
	Access    []AccessEntry `json:"access"`
}

// GenerateDockerToken creates a signed JWT for Docker CLI authorization.
func GenerateDockerToken(signingSecret []byte, subject, audience string, access []AccessEntry, ttl time.Duration) (string, error) {
	now := time.Now().Unix()
	claims := TokenClaims{
		Issuer:    "renop",
		Subject:   subject,
		Audience:  audience,
		ExpiresAt: now + int64(ttl.Seconds()),
		NotBefore: now - 5,
		IssuedAt:  now,
		Access:    access,
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)

	message := dockerTokenHeader + "." + claimsEncoded
	mac := hmac.New(sha256.New, signingSecret)
	mac.Write([]byte(message))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return message + "." + signature, nil
}

// ValidateDockerToken verifies the HMAC signature and expiration of a Docker bearer token.
func ValidateDockerToken(signingSecret []byte, tokenStr string) (*TokenClaims, error) {
	headerEncoded, remainder, ok := strings.Cut(tokenStr, ".")
	if !ok {
		return nil, errors.New("invalid token format")
	}
	claimsEncoded, signatureEncoded, ok := strings.Cut(remainder, ".")
	if !ok || headerEncoded == "" || claimsEncoded == "" || signatureEncoded == "" || strings.Contains(signatureEncoded, ".") {
		return nil, errors.New("invalid token format")
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(signatureEncoded)
	if err != nil {
		return nil, errors.New("invalid token signature")
	}

	message := headerEncoded + "." + claimsEncoded
	mac := hmac.New(sha256.New, signingSecret)
	mac.Write([]byte(message))

	if !hmac.Equal(providedSignature, mac.Sum(nil)) {
		return nil, errors.New("invalid token signature")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(claimsEncoded)
	if err != nil {
		return nil, errors.New("invalid claims encoding")
	}

	var claims TokenClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, errors.New("invalid claims JSON")
	}

	now := time.Now().Unix()
	if claims.ExpiresAt < now {
		return nil, errors.New("token expired")
	}
	if claims.NotBefore > now+60 {
		return nil, errors.New("token not yet valid")
	}

	return &claims, nil
}

// HandleTokenAuth handles the /v2/token and /v2/auth endpoints.
func HandleTokenAuth(c fiber.Ctx, state *core.AppState) error {
	service := c.Query("service")
	account := c.Query("account")
	if account == "" {
		account = c.Query("client_id")
	}

	cfg := state.Inner.Config.Load()
	if service == "" {
		service = cfg.Server.Host
	}

	user := auth.GetUser(c)
	var directCredential *auth.VerifiedCredential
	if user == nil || user.Username == "guest" {
		authHeader := c.Get(fiber.HeaderAuthorization)
		if after, ok := strings.CutPrefix(authHeader, "Basic "); ok {
			basicAuth := after
			if decoded, err := utils.DecodeB64(basicAuth); err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					u := strings.ToLower(parts[0])
					p := parts[1]
					if tokenObj := state.GetTokenByName(u); tokenObj != nil {
						if credential := verifyTokenSecretDirect(state, tokenObj, p); credential != nil {
							user = &config.User{
								Username: u,
								Roles:    tokenObj.Permissions,
							}
							directCredential = credential
						}
					}
				}
			}
		}

		if (user == nil || user.Username == "guest") && account != "" {
			password := c.Query("password")
			if password != "" {
				u := strings.ToLower(account)
				if tokenObj := state.GetTokenByName(u); tokenObj != nil {
					if credential := verifyTokenSecretDirect(state, tokenObj, password); credential != nil {
						user = &config.User{
							Username: u,
							Roles:    tokenObj.Permissions,
						}
						directCredential = credential
					}
				}
			}
		}
	}

	if user == nil {
		user = auth.GuestUser
	}

	var rawScopes []string
	for k, v := range c.Request().URI().QueryArgs().All() {
		if string(k) == "scope" {
			rawScopes = append(rawScopes, strings.Fields(string(v))...)
		}
	}
	if len(rawScopes) == 0 {
		if s := c.Query("scope"); s != "" {
			rawScopes = strings.Fields(s)
		}
	}

	var accessList []AccessEntry
	credentialAllows := func(scope, target string) bool {
		if directCredential != nil {
			return directCredential.HasScopeTarget(scope, target)
		}
		return auth.CurrentCredentialHasScopeTarget(c, scope, target)
	}
	for _, scope := range rawScopes {
		scopeParts := strings.SplitN(scope, ":", 3)
		if len(scopeParts) == 3 && scopeParts[0] == "repository" {
			repoFullName := scopeParts[1]
			actionsReq := strings.Split(scopeParts[2], ",")

			repoName, _ := ParseRepositoryAndImage(repoFullName)
			repo := cfg.Maven.Repositories[repoName]
			canPullWithCredential := credentialAllows(core.APITokenScopeRepositoryRead, repoName)
			canPushWithCredential := credentialAllows(core.APITokenScopeRepositoryPublish, repoName)
			canDeleteWithCredential := credentialAllows(core.APITokenScopeRepositoryDelete, repoName)

			var grantedActions []string
			appendGranted := func(action string) {
				if !slices.Contains(grantedActions, action) {
					grantedActions = append(grantedActions, action)
				}
			}
			writeChecked := false
			canWriteResource := false
			checkWrite := func() bool {
				if !writeChecked {
					canWriteResource = CanWriteDocker(state, user, repo, repoFullName)
					writeChecked = true
				}
				return canWriteResource
			}
			for _, action := range actionsReq {
				action = strings.TrimSpace(action)
				switch action {
				case "pull":
					if canPullWithCredential && CanReadDocker(state, user, repo, repoFullName) {
						appendGranted("pull")
					}
				case "push":
					if canPushWithCredential && checkWrite() {
						appendGranted("push")
					}
					if canDeleteWithCredential && checkWrite() {
						appendGranted("delete")
					}
				case "delete":
					if canDeleteWithCredential && checkWrite() {
						appendGranted("delete")
					}
				}
			}

			accessList = append(accessList, AccessEntry{
				Type:    "repository",
				Name:    repoFullName,
				Actions: grantedActions,
			})
		}
	}

	signingKey := state.GetDockerSecret()
	tokenStr, err := GenerateDockerToken(signingKey, user.Username, service, accessList, 2*time.Hour)
	if err != nil {
		return RespondError(c, fiber.StatusInternalServerError, ErrCodeDenied, "failed to issue token", nil)
	}

	c.Set(DockerHeaderVersion, DockerVersionValue)
	c.Set(fiber.HeaderContentType, "application/json; charset=utf-8")
	return c.Status(fiber.StatusOK).JSON(TokenResponse{
		Token:       tokenStr,
		AccessToken: tokenStr,
		ExpiresIn:   7200,
		IssuedAt:    time.Now().UTC().Format(time.RFC3339),
	})
}

func verifyTokenSecretDirect(state *core.AppState, accessToken *core.AccessToken, secret string) *auth.VerifiedCredential {
	credential, err := auth.VerifyAccountCredential(state, accessToken, secret)
	if err != nil {
		return nil
	}
	return credential
}
