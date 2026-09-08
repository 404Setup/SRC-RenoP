/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"bytes"
	"context"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"renop/internal/config"
	"renop/internal/core"
)

type oauthTokens struct {
	AccessToken string          `json:"access_token"`
	IDToken     string          `json:"id_token"`
	TokenType   string          `json:"token_type"`
	Error       json.RawMessage `json:"error"`
}

type oauthUserInfo struct {
	Identity      core.OAuthIdentity
	Username      string
	Name          string
	Email         string
	EmailVerified bool
	AvatarURL     string
}

func exchangeOAuthCode(ctx context.Context, client *http.Client, p config.OAuthProviderConfig, code, verifier string) (oauthTokens, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {p.ClientID}, "code": {code}, "redirect_uri": {p.CallbackURL}}
	if !p.DisablePKCE {
		form.Set("code_verifier", verifier)
	}
	if p.TokenAuth == "client_secret_post" {
		form.Set("client_secret", p.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokens{}, errors.New("OAuth token request is invalid")
	}
	if p.TokenAuth == "client_secret_basic" {
		req.SetBasicAuth(url.QueryEscape(p.ClientID), url.QueryEscape(p.ClientSecret))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "RenoP-OAuth/1")
	response, err := client.Do(req)
	if err != nil {
		return oauthTokens{}, errors.New("OAuth token request failed")
	}
	var tokens oauthTokens
	if err = decodeOAuthResponse(response, &tokens); err != nil {
		return oauthTokens{}, err
	}
	if len(tokens.Error) != 0 && string(tokens.Error) != "null" || tokens.AccessToken == "" || len(tokens.AccessToken) > 16384 ||
		strings.ContainsAny(tokens.AccessToken, "\r\n\x00") || len(tokens.IDToken) > 16384 ||
		tokens.TokenType != "" && !strings.EqualFold(tokens.TokenType, "bearer") || tokens.TokenType == "" && p.Type != "stackexchange" {
		return oauthTokens{}, errors.New("OAuth provider rejected the authorization code")
	}
	return tokens, nil
}

func getOAuthJSON(ctx context.Context, client *http.Client, endpoint, accessToken string, destination any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("OAuth identity request is invalid")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "RenoP-OAuth/1")
	response, err := client.Do(req)
	if err != nil {
		return errors.New("OAuth identity request failed")
	}
	return decodeOAuthResponse(response, destination)
}

func oauthClaim(value json.RawMessage, path string) json.RawMessage {
	if path == "" || len(path) > 128 {
		return nil
	}
	for _, part := range strings.Split(path, ".") {
		value = bytes.TrimSpace(value)
		if len(value) == 0 {
			return nil
		}
		if value[0] == '[' {
			index, err := strconv.Atoi(part)
			var items []json.RawMessage
			if err != nil || index < 0 || json.Unmarshal(value, &items) != nil || index >= len(items) {
				return nil
			}
			value = items[index]
		} else {
			var fields map[string]json.RawMessage
			if json.Unmarshal(value, &fields) != nil {
				return nil
			}
			value = fields[part]
		}
	}
	return bytes.TrimSpace(value)
}

func oauthString(value json.RawMessage, path string, numeric bool) string {
	value = oauthClaim(value, path)
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	if numeric && len(value) > 0 && len(value) <= 255 {
		for _, r := range value {
			if r < '0' || r > '9' {
				return ""
			}
		}
		return string(value)
	}
	return ""
}

func fetchOAuthUserInfo(ctx context.Context, client *http.Client, p config.OAuthProviderConfig, tokens oauthTokens, nonce string) (oauthUserInfo, error) {
	issuer := p.Issuer
	subject := ""
	if issuer != "" {
		claims, err := verifyOAuthIDToken(ctx, client, p, tokens, nonce)
		if err != nil {
			return oauthUserInfo{}, err
		}
		issuer, _ = claims.GetIssuer()
		if p.Type == "google" && issuer == "accounts.google.com" {
			issuer = p.Issuer
		}
		subject, _ = claims.GetSubject()
	}
	endpoint := p.UserInfoURL
	if p.Type == "stackexchange" {
		u, _ := url.Parse(endpoint)
		query := u.Query()
		query.Set("site", p.Site)
		query.Set("key", p.APIKey)
		query.Set("access_token", tokens.AccessToken)
		query.Set("pagesize", "1")
		u.RawQuery = query.Encode()
		endpoint = u.String()
	}
	var raw json.RawMessage
	if err := getOAuthJSON(ctx, client, endpoint, tokens.AccessToken, &raw); err != nil {
		return oauthUserInfo{}, err
	}
	id := oauthString(raw, p.Claims.Subject, true)
	if p.Type == "stackexchange" {
		accountID, err := strconv.ParseInt(id, 10, 64)
		if err != nil || accountID <= 0 {
			return oauthUserInfo{}, errors.New("Stack Exchange returned an invalid network account")
		}
	}
	if subject != "" && id != subject {
		return oauthUserInfo{}, errors.New("OAuth user-info subject does not match the ID token")
	}
	info := oauthUserInfo{Identity: core.OAuthIdentity{ProviderID: p.ID, Subject: id, Authority: p.Authority(issuer)},
		Username: oauthString(raw, p.Claims.Username, false), Name: oauthString(raw, p.Claims.Name, false),
		Email: oauthString(raw, p.Claims.Email, false), AvatarURL: oauthString(raw, p.Claims.Avatar, false)}
	info.Email, _ = core.NormalizeEmail(info.Email)
	info.EmailVerified = info.Email != "" && string(oauthClaim(raw, p.Claims.EmailVerified)) == "true"
	if p.Type == "gitlab" && p.UserInfoURL == "https://gitlab.com/oauth/userinfo" {
		info.Identity.Namespaces = gitlabOwnedNamespaces(raw)
	}
	if p.Type == "stackexchange" {
		info.Name = html.UnescapeString(info.Name)
	}
	info.Identity.Login = info.Username
	if info.Identity.Login == "" {
		info.Identity.Login = info.Name
	}
	if len(info.Identity.Login) > 255 || strings.ContainsAny(info.Identity.Login, "\x00\r\n") {
		info.Identity.Login = ""
	}
	if !info.Identity.Valid() {
		return oauthUserInfo{}, errors.New("OAuth provider returned an invalid subject")
	}
	if !config.ValidOAuthURL(info.AvatarURL) {
		info.AvatarURL = ""
	}
	return info, nil
}

func gitlabOwnedNamespaces(raw json.RawMessage) []string {
	var claims map[string]json.RawMessage
	if json.Unmarshal(raw, &claims) != nil {
		return nil
	}
	var owned []string
	_ = json.Unmarshal(claims["https://gitlab.org/claims/groups/owner"], &owned)
	namespaces := []string{}
	seen := map[string]bool{}
	for _, name := range append([]string{oauthString(raw, "preferred_username", false)}, owned...) {
		name = strings.ToLower(name)
		// Owning a subgroup does not prove control over its parent namespace.
		if name == "" || len(name) > 63 || strings.ContainsAny(name, "/.\x00\r\n") || seen[name] {
			continue
		}
		namespaces = append(namespaces, name)
		seen[name] = true
		if len(namespaces) == 1001 {
			break
		}
	}
	return namespaces
}
