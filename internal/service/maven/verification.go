/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

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
	verificationTimeout  = 10 * time.Second
	verificationBodySize = 64 << 10
)

var verificationSemaphore = make(chan struct{}, 8)

var errVerificationAccountMissing = errors.New("verification account was not found")

var verificationClient = func(proxyConfig *config.OutboundProxy) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   4 * time.Second,
			KeepAlive: -1,
		}).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		DisableCompression:     true,
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    4 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		MaxResponseHeaderBytes: 128 << 10,
	}
	if proxyConfig != nil {
		if err := outboundproxy.ConfigureTransport(transport, proxyConfig); err != nil {
			return nil, err
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   verificationTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func readVerificationResponse(response *http.Response, destination any) error {
	if response == nil {
		return errors.New("verification response is missing")
	}
	defer utils.DiscardHTTPBody(response.Body, response.ContentLength)
	if response.StatusCode == http.StatusNotFound {
		return errVerificationAccountMissing
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("verification provider returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > verificationBodySize {
		return errors.New("verification response exceeds the size limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, verificationBodySize+1))
	if err != nil {
		return err
	}
	if len(body) > verificationBodySize {
		return errors.New("verification response exceeds the size limit")
	}
	return json.Unmarshal(body, destination)
}

func getVerificationJSON(ctx context.Context, client *http.Client, endpoint string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "RenoP-Maven-Domain-Verifier/1")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Close = true
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	return readVerificationResponse(response, destination)
}

func verifyDNS(ctx context.Context, host, code string) error {
	records, err := net.DefaultResolver.LookupTXT(ctx, host)
	if err != nil {
		var dnsError *net.DNSError
		if errors.As(err, &dnsError) && dnsError.IsNotFound {
			return core.ErrMavenVerificationFailed
		}
		return fmt.Errorf("lookup Maven verification TXT: %w", err)
	}
	if verificationTXTMatches(records, code) {
		return nil
	}
	return core.ErrMavenVerificationFailed
}

func verificationTXTMatches(records []string, code string) bool {
	for _, record := range records {
		if strings.TrimSpace(record) == code {
			return true
		}
	}
	return false
}

type verificationProfile struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Login       string `json:"login"`
	Username    string `json:"username"`
	FullPath    string `json:"full_path"`
	Bio         string `json:"bio"`
	Description string `json:"description"`
}

func providerIdentity(ctx context.Context, client *http.Client, provider, account, accountType, code string) (*core.MavenDomainHealth, error) {
	var profile verificationProfile
	if provider == core.MavenVerificationGitHub {
		if err := getVerificationJSON(ctx, client, "https://api.github.com/users/"+url.PathEscape(account), &profile); err != nil {
			return nil, err
		}
		if !strings.EqualFold(profile.Login, account) {
			return nil, errVerificationAccountMissing
		}
		profile.Type = strings.ToLower(profile.Type)
		if profile.Type == core.GitHubPrincipalOrganization && code != "" {
			id := profile.ID
			if err := getVerificationJSON(ctx, client, "https://api.github.com/orgs/"+url.PathEscape(account), &profile); err != nil {
				return nil, err
			}
			if profile.ID != id || !strings.EqualFold(profile.Login, account) {
				return nil, core.ErrMavenVerificationFailed
			}
			profile.Type = core.GitHubPrincipalOrganization
		}
	} else if provider == core.MavenVerificationGitLab {
		if accountType != core.GitHubPrincipalUser {
			err := getVerificationJSON(ctx, client, "https://gitlab.com/api/v4/groups/"+url.PathEscape(account)+"?with_projects=false", &profile)
			if err != nil && !errors.Is(err, errVerificationAccountMissing) {
				return nil, err
			}
			if err == nil {
				if !strings.EqualFold(profile.FullPath, account) {
					return nil, errVerificationAccountMissing
				}
				profile.Type = core.GitHubPrincipalOrganization
			} else if accountType == core.GitHubPrincipalOrganization {
				return nil, err
			}
		}
		if profile.Type == "" {
			var users []verificationProfile
			query := url.Values{"username": {account}, "per_page": {"2"}}
			if err := getVerificationJSON(ctx, client, "https://gitlab.com/api/v4/users?"+query.Encode(), &users); err != nil {
				return nil, err
			}
			if len(users) != 1 || !strings.EqualFold(users[0].Username, account) {
				return nil, errVerificationAccountMissing
			}
			profile = users[0]
			profile.Type = core.GitHubPrincipalUser
		}
	} else {
		return nil, errors.New("unsupported Maven account provider")
	}
	if profile.ID <= 0 || (profile.Type != core.GitHubPrincipalUser && profile.Type != core.GitHubPrincipalOrganization) {
		return nil, errors.New("verification provider returned an invalid identity")
	}
	if code != "" && !strings.Contains(profile.Bio, code) && !strings.Contains(profile.Description, code) {
		return nil, core.ErrMavenVerificationFailed
	}
	return &core.MavenDomainHealth{ProviderID: strconv.FormatInt(profile.ID, 10), ProviderType: profile.Type, Status: "active"}, nil
}

// VerifyDomainProof checks the fixed external proof target assigned to a domain.
func VerifyDomainProof(ctx context.Context, cfg *config.Config, domain *core.MavenDomain) (*core.MavenDomainHealth, error) {
	if cfg == nil || domain == nil || domain.VerificationCode == "" {
		return nil, errors.New("maven domain verification configuration is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, verificationTimeout)
	defer cancel()
	select {
	case verificationSemaphore <- struct{}{}:
		defer func() { <-verificationSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if domain.VerificationType == core.MavenVerificationDNS {
		if err := verifyDNS(ctx, domain.VerificationHost, domain.VerificationCode); err != nil {
			return nil, err
		}
	}
	proxyConfig, err := outboundproxy.Selected(cfg.Proxy)
	if err != nil {
		return nil, err
	}
	client, err := verificationClient(proxyConfig)
	if err != nil {
		return nil, err
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	health, err := inspectDomainHealth(ctx, client, domain, domain.VerificationCode)
	if err != nil {
		return nil, err
	}
	if health.Status != "active" {
		return nil, core.ErrMavenVerificationFailed
	}
	return health, nil
}
