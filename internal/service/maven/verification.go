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

func verificationClient(proxyConfig *config.OutboundProxy) (*http.Client, error) {
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
	if response.StatusCode != http.StatusOK {
		return core.ErrMavenVerificationFailed
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

func verifyGitHub(ctx context.Context, client *http.Client, account, code string) error {
	var profile struct {
		Bio         string `json:"bio"`
		Description string `json:"description"`
	}
	endpoint := "https://api.github.com/users/" + url.PathEscape(account)
	if err := getVerificationJSON(ctx, client, endpoint, &profile); err == nil {
		if strings.Contains(profile.Bio, code) || strings.Contains(profile.Description, code) {
			return nil
		}
	}
	profile = struct {
		Bio         string `json:"bio"`
		Description string `json:"description"`
	}{}
	endpoint = "https://api.github.com/orgs/" + url.PathEscape(account)
	if err := getVerificationJSON(ctx, client, endpoint, &profile); err != nil {
		return core.ErrMavenVerificationFailed
	}
	if strings.Contains(profile.Description, code) || strings.Contains(profile.Bio, code) {
		return nil
	}
	return core.ErrMavenVerificationFailed
}

func verifyGitLab(ctx context.Context, client *http.Client, account, code string) error {
	var group struct {
		Description string `json:"description"`
	}
	endpoint := "https://gitlab.com/api/v4/groups/" + url.PathEscape(account)
	if err := getVerificationJSON(ctx, client, endpoint, &group); err == nil && strings.Contains(group.Description, code) {
		return nil
	}
	var users []struct {
		Bio string `json:"bio"`
	}
	query := url.Values{"username": []string{account}}
	endpoint = "https://gitlab.com/api/v4/users?" + query.Encode()
	if err := getVerificationJSON(ctx, client, endpoint, &users); err == nil {
		for _, user := range users {
			if strings.Contains(user.Bio, code) {
				return nil
			}
		}
	}
	return core.ErrMavenVerificationFailed
}

// VerifyDomainProof checks the fixed external proof target assigned to a domain.
func VerifyDomainProof(ctx context.Context, cfg *config.Config, domain *core.MavenDomain) error {
	if cfg == nil || domain == nil || domain.VerificationCode == "" {
		return errors.New("Maven domain verification configuration is unavailable")
	}
	select {
	case verificationSemaphore <- struct{}{}:
		defer func() { <-verificationSemaphore }()
	case <-ctx.Done():
		return ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, verificationTimeout)
	defer cancel()
	if domain.VerificationType == core.MavenVerificationDNS {
		return verifyDNS(ctx, domain.VerificationHost, domain.VerificationCode)
	}
	proxyConfig, err := outboundproxy.Selected(cfg.Proxy)
	if err != nil {
		return err
	}
	client, err := verificationClient(proxyConfig)
	if err != nil {
		return err
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	switch domain.VerificationType {
	case core.MavenVerificationGitHub:
		return verifyGitHub(ctx, client, domain.VerificationHost, domain.VerificationCode)
	case core.MavenVerificationGitLab:
		return verifyGitLab(ctx, client, domain.VerificationHost, domain.VerificationCode)
	default:
		return errors.New("unsupported Maven verification method")
	}
}
