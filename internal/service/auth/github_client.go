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
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/outboundproxy"
	"renop/internal/utils"
)

const (
	githubOAuthResponseSize = 1 << 20
	githubOAuthOrgPageSize  = 100
	githubOAuthMaxOrgPages  = 10
)

type githubOAuthProvider struct {
	AuthorizeURL string
	TokenURL     string
	APIURL       string
	AvatarURL    string
}

var defaultGitHubOAuthProvider = githubOAuthProvider{
	AuthorizeURL: "https://github.com/login/oauth/authorize",
	TokenURL:     "https://github.com/login/oauth/access_token",
	APIURL:       "https://api.github.com",
	AvatarURL:    "https://avatars.githubusercontent.com/u",
}

type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

type githubAPIIdentity struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

func githubOAuthHTTPClient(cfg *config.Config) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  8 * time.Second,
		MaxResponseHeaderBytes: 128 << 10,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        4,
		IdleConnTimeout:        30 * time.Second,
	}
	proxyConfig, err := outboundproxy.Selected(cfg.Proxy)
	if err != nil {
		return nil, err
	}
	if proxyConfig != nil {
		if err := outboundproxy.ConfigureTransport(transport, proxyConfig); err != nil {
			return nil, err
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func decodeGitHubResponse(response *http.Response, destination any) error {
	if response == nil {
		return errors.New("GitHub response is missing")
	}
	defer utils.DiscardHTTPBody(response.Body, response.ContentLength)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > githubOAuthResponseSize {
		return errors.New("GitHub response exceeds the size limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, githubOAuthResponseSize+1))
	if err != nil {
		return err
	}
	if len(body) > githubOAuthResponseSize {
		return errors.New("GitHub response exceeds the size limit")
	}
	return json.Unmarshal(body, destination)
}

func exchangeGitHubCode(ctx context.Context, client *http.Client, provider githubOAuthProvider,
	cfg config.GitHubOAuthConfig, code string) (githubTokenResponse, error) {
	form := url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"code":          {code},
		"redirect_uri":  {cfg.CallbackURL},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return githubTokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "RenoP-GitHub-OAuth/1")
	response, err := client.Do(request)
	if err != nil {
		return githubTokenResponse{}, err
	}
	var tokenResponse githubTokenResponse
	if err := decodeGitHubResponse(response, &tokenResponse); err != nil {
		return githubTokenResponse{}, err
	}
	if tokenResponse.Error != "" || tokenResponse.AccessToken == "" ||
		!strings.EqualFold(tokenResponse.TokenType, "bearer") {
		return githubTokenResponse{}, errors.New("GitHub rejected the authorization code")
	}
	return tokenResponse, nil
}

func githubScopesAuthorized(scopeValue string) bool {
	scopes := make(map[string]struct{})
	for _, scope := range strings.FieldsFunc(strings.ToLower(scopeValue), func(character rune) bool {
		return character == ',' || unicodeSpace(character)
	}) {
		scopes[strings.TrimSpace(scope)] = struct{}{}
	}
	_, fullUser := scopes["user"]
	_, readUser := scopes["read:user"]
	_, readOrg := scopes["read:org"]
	_, writeOrg := scopes["write:org"]
	_, adminOrg := scopes["admin:org"]
	return (fullUser || readUser) && (readOrg || writeOrg || adminOrg)
}

func unicodeSpace(character rune) bool {
	switch character {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func getGitHubAPI(ctx context.Context, client *http.Client, endpoint, accessToken string,
	destination any) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("User-Agent", "RenoP-GitHub-OAuth/1")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	linkHeader := response.Header.Get("Link")
	if err := decodeGitHubResponse(response, destination); err != nil {
		return "", err
	}
	return linkHeader, nil
}

func fetchGitHubAuthorization(ctx context.Context, client *http.Client, provider githubOAuthProvider,
	accessToken string) (githubAPIIdentity, []core.GitHubPrincipal, error) {
	var user githubAPIIdentity
	if _, err := getGitHubAPI(ctx, client, strings.TrimRight(provider.APIURL, "/")+"/user",
		accessToken, &user); err != nil {
		return githubAPIIdentity{}, nil, err
	}
	user.Login = strings.ToLower(strings.TrimSpace(user.Login))
	if user.ID <= 0 || user.Login == "" || len(user.Login) > 39 {
		return githubAPIIdentity{}, nil, errors.New("GitHub returned an invalid user identity")
	}
	principals := make([]core.GitHubPrincipal, 0, 8)
	principals = append(principals, core.GitHubPrincipal{
		Type: core.GitHubPrincipalUser, GitHubID: user.ID, Login: user.Login,
	})
	for page := 1; page <= githubOAuthMaxOrgPages; page++ {
		var organizations []githubAPIIdentity
		endpoint := strings.TrimRight(provider.APIURL, "/") + "/user/orgs?per_page=" +
			strconv.Itoa(githubOAuthOrgPageSize) + "&page=" + strconv.Itoa(page)
		linkHeader, err := getGitHubAPI(ctx, client, endpoint, accessToken, &organizations)
		if err != nil {
			return githubAPIIdentity{}, nil, err
		}
		for _, organization := range organizations {
			organization.Login = strings.ToLower(strings.TrimSpace(organization.Login))
			if organization.ID <= 0 || organization.Login == "" || len(organization.Login) > 39 {
				return githubAPIIdentity{}, nil, errors.New("GitHub returned an invalid organization identity")
			}
			principals = append(principals, core.GitHubPrincipal{
				Type: core.GitHubPrincipalOrganization, GitHubID: organization.ID, Login: organization.Login,
			})
		}
		if len(organizations) < githubOAuthOrgPageSize || !strings.Contains(linkHeader, `rel="next"`) {
			return user, principals, nil
		}
	}
	return githubAPIIdentity{}, nil, errors.New("GitHub organization list exceeds the supported limit")
}
