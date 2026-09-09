/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// OAuthClaims selects scalar fields from a JSON user-info response using dotted paths and numeric array indexes.
type OAuthClaims struct {
	Subject       string `json:"subject" yaml:"subject"`
	Username      string `json:"username" yaml:"username"`
	Name          string `json:"name" yaml:"name"`
	Email         string `json:"email" yaml:"email"`
	EmailVerified string `json:"email_verified" yaml:"email_verified"`
	Avatar        string `json:"avatar" yaml:"avatar"`
}

// OAuthProviderConfig describes one administrator-managed OAuth client; credentials are never public settings.
type OAuthProviderConfig struct {
	ID           string      `json:"id" yaml:"id"`
	Type         string      `json:"type" yaml:"type"`
	Name         string      `json:"name" yaml:"name"`
	Enabled      bool        `json:"enabled" yaml:"enabled"`
	ClientID     string      `json:"client_id" yaml:"client_id"`
	ClientSecret string      `json:"client_secret" yaml:"client_secret"`
	CallbackURL  string      `json:"callback_url" yaml:"callback_url"`
	Tenant       string      `json:"tenant" yaml:"tenant"`
	BaseURL      string      `json:"base_url" yaml:"base_url"`
	Site         string      `json:"site" yaml:"site"`
	APIKey       string      `json:"api_key" yaml:"api_key"`
	AuthorizeURL string      `json:"authorize_url" yaml:"authorize_url"`
	TokenURL     string      `json:"token_url" yaml:"token_url"`
	UserInfoURL  string      `json:"userinfo_url" yaml:"userinfo_url"`
	Issuer       string      `json:"issuer" yaml:"issuer"`
	JWKSURL      string      `json:"jwks_url" yaml:"jwks_url"`
	Scopes       string      `json:"scopes" yaml:"scopes"`
	TokenAuth    string      `json:"token_auth" yaml:"token_auth"`
	DisablePKCE  bool        `json:"disable_pkce" yaml:"disable_pkce"`
	Claims       OAuthClaims `json:"claims" yaml:"claims"`
}

var oauthIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var oauthTenantPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)
var oauthClaimPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+(?:\.[a-zA-Z0-9_-]+)*$`)

// ValidOAuthURL permits HTTPS services and loopback HTTP development endpoints without URL credentials.
func ValidOAuthURL(value string) bool {
	if value == "" || len(value) > 2048 || strings.ContainsAny(value, "\\\r\n\x00") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
}

// Resolved applies official provider endpoints while retaining administrator-selected credentials and instance settings.
func (p OAuthProviderConfig) Resolved() OAuthProviderConfig {
	if p.TokenAuth == "" {
		p.TokenAuth = "client_secret_post"
	}
	if p.Type != "custom" {
		p.Claims = OAuthClaims{Subject: "sub", Username: "preferred_username", Name: "name", Email: "email", EmailVerified: "email_verified", Avatar: "picture"}
		p.DisablePKCE = false
	}
	defaultScopes := "openid profile email"
	switch p.Type {
	case "microsoft":
		if p.Tenant == "" {
			p.Tenant = "common"
		}
		base := "https://login.microsoftonline.com/" + p.Tenant
		p.AuthorizeURL, p.TokenURL = base+"/oauth2/v2.0/authorize", base+"/oauth2/v2.0/token"
		p.UserInfoURL, p.JWKSURL = "https://graph.microsoft.com/oidc/userinfo", base+"/discovery/v2.0/keys"
		p.Issuer = "https://login.microsoftonline.com/{tenantid}/v2.0"
		p.Claims.EmailVerified = ""
	case "google":
		p.AuthorizeURL, p.TokenURL = "https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token"
		p.UserInfoURL, p.JWKSURL = "https://openidconnect.googleapis.com/v1/userinfo", "https://www.googleapis.com/oauth2/v3/certs"
		p.Issuer = "https://accounts.google.com"
	case "gitlab":
		if p.BaseURL == "" {
			p.BaseURL = "https://gitlab.com"
		}
		base := strings.TrimRight(p.BaseURL, "/")
		p.AuthorizeURL, p.TokenURL = base+"/oauth/authorize", base+"/oauth/token"
		p.UserInfoURL, p.JWKSURL, p.Issuer = base+"/oauth/userinfo", base+"/oauth/discovery/keys", base
	case "cloudflare":
		p.AuthorizeURL, p.TokenURL = "https://dash.cloudflare.com/oauth2/auth", "https://dash.cloudflare.com/oauth2/token"
		p.UserInfoURL, p.JWKSURL = "https://dash.cloudflare.com/oauth2/userinfo", "https://dash.cloudflare.com/.well-known/jwks.json"
		p.Issuer, defaultScopes = "https://dash.cloudflare.com", "openid"
		p.Claims = OAuthClaims{Subject: "sub"}
	case "stackexchange":
		if p.Site == "" {
			p.Site = "stackoverflow"
		}
		p.AuthorizeURL, p.TokenURL = "https://stackoverflow.com/oauth", "https://stackoverflow.com/oauth/access_token/json"
		p.UserInfoURL, p.Issuer, p.JWKSURL = "https://api.stackexchange.com/2.3/me", "", ""
		p.Claims = OAuthClaims{Subject: "items.0.account_id", Name: "items.0.display_name", Avatar: "items.0.profile_image"}
		defaultScopes = ""
	case "custom":
		defaultScopes = ""
	}
	if p.Scopes == "" {
		p.Scopes = defaultScopes
	}
	return p
}

// Authority binds stable subjects to the client, identity endpoint, and verified issuer across configuration changes.
func (p OAuthProviderConfig) Authority(issuer string) string {
	p = p.Resolved()
	parts := []string{p.Type, p.ClientID, p.AuthorizeURL, p.TokenURL, p.UserInfoURL, issuer}
	// OIDC verifies sub; OAuth-only JSON selectors define their own subject namespace.
	if p.Type == "custom" && issuer == "" {
		parts = append(parts, p.Claims.Subject)
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:])
}

// Validate checks provider bounds, endpoint security, and the prerequisites for enabled clients.
func (p OAuthProviderConfig) Validate() error {
	invalid := errors.New("OAuth provider configuration is invalid")
	if !oauthIDPattern.MatchString(p.ID) || p.ID == "github" || p.Name == "" || len(p.Name) > 80 ||
		!slices.Contains([]string{"microsoft", "google", "gitlab", "cloudflare", "stackexchange", "custom"}, p.Type) {
		return invalid
	}
	for _, value := range []string{p.Name, p.ClientID, p.ClientSecret, p.APIKey, p.Scopes} {
		if len(value) > 4096 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return invalid
		}
	}
	if len(p.ClientID) > 512 || len(p.Scopes) > 1024 || p.Tenant != "" && !oauthTenantPattern.MatchString(p.Tenant) ||
		p.Site != "" && !oauthTenantPattern.MatchString(p.Site) {
		return invalid
	}
	if p.Type == "microsoft" && p.Tenant != "" && !slices.Contains([]string{"common", "organizations", "consumers"}, p.Tenant) {
		tenant, err := uuid.Parse(p.Tenant)
		if err != nil || tenant.String() != strings.ToLower(p.Tenant) {
			return invalid
		}
	}
	if p.BaseURL != "" {
		u, err := url.Parse(p.BaseURL)
		if err != nil || !ValidOAuthURL(p.BaseURL) || u.RawQuery != "" || u.RawPath != "" {
			return invalid
		}
	}
	if p.CallbackURL != "" {
		u, err := url.Parse(p.CallbackURL)
		if err != nil || !ValidOAuthURL(p.CallbackURL) || u.RawQuery != "" || u.RawPath != "" || u.Path != "/api/auth/oauth/"+p.ID+"/callback" {
			return invalid
		}
	}
	p = p.Resolved()
	if !slices.Contains([]string{"client_secret_post", "client_secret_basic", "none"}, p.TokenAuth) || p.DisablePKCE && p.TokenAuth == "none" {
		return invalid
	}
	for _, value := range []string{p.AuthorizeURL, p.TokenURL, p.UserInfoURL, p.JWKSURL} {
		if value != "" && !ValidOAuthURL(value) {
			return invalid
		}
	}
	for _, path := range []string{p.Claims.Subject, p.Claims.Username, p.Claims.Name, p.Claims.Email, p.Claims.EmailVerified, p.Claims.Avatar} {
		if path != "" && (len(path) > 128 || !oauthClaimPattern.MatchString(path)) {
			return invalid
		}
	}
	if p.Type == "custom" && p.Issuer != "" && (!ValidOAuthURL(p.Issuer) || p.JWKSURL == "") {
		return invalid
	}
	if p.Type == "custom" && slices.Contains(strings.Fields(p.Scopes), "openid") && p.Issuer == "" {
		return invalid
	}
	if p.Enabled && (p.ClientID == "" || p.CallbackURL == "" || p.TokenAuth != "none" && p.ClientSecret == "" ||
		p.AuthorizeURL == "" || p.TokenURL == "" || p.UserInfoURL == "" || p.Claims.Subject == "" ||
		p.Type == "stackexchange" && p.APIKey == "") {
		return invalid
	}
	if p.Enabled && p.Issuer != "" && !slices.Contains(strings.Fields(p.Scopes), "openid") {
		return invalid
	}
	return nil
}

// Configured reports whether this provider can safely accept new authorization requests.
func (p OAuthProviderConfig) Configured() bool { return p.Enabled && p.Validate() == nil }

// OAuthProvider returns a configured provider by its stable local identifier.
func (s ServerConfig) OAuthProvider(id string) (OAuthProviderConfig, bool) {
	if len(s.OAuthProviders) > 32 {
		return OAuthProviderConfig{}, false
	}
	for _, p := range s.OAuthProviders {
		if p.ID == id && p.Configured() {
			return p.Resolved(), true
		}
	}
	return OAuthProviderConfig{}, false
}
