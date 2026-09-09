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
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"renop/internal/core"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verificationRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip verificationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func verificationResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    &http.Request{},
	}
}

func TestGitHubAndGitLabVerificationProfiles(t *testing.T) {
	const code = "renop-verification=current"
	client := &http.Client{Transport: verificationRoundTripper(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "api.github.com/users/example":
			return verificationResponse(http.StatusOK, `{"id":42,"login":"example","type":"Organization","bio":"unrelated"}`), nil
		case "api.github.com/orgs/example":
			return verificationResponse(http.StatusOK, `{"id":42,"login":"example","description":"proof renop-verification=current"}`), nil
		case "gitlab.com/api/v4/groups/example":
			return verificationResponse(http.StatusNotFound, `{}`), nil
		case "gitlab.com/api/v4/users":
			assert.Equal(t, "example", request.URL.Query().Get("username"))
			return verificationResponse(http.StatusOK, `[{"id":71,"username":"example","bio":"renop-verification=current"}]`), nil
		default:
			return verificationResponse(http.StatusNotFound, `{}`), nil
		}
	})}
	github, err := providerIdentity(context.Background(), client, core.MavenVerificationGitHub, "example", "", code)
	require.NoError(t, err)
	require.Equal(t, "42", github.ProviderID)
	require.Equal(t, core.GitHubPrincipalOrganization, github.ProviderType)
	gitlab, err := providerIdentity(context.Background(), client, core.MavenVerificationGitLab, "example", "", code)
	require.NoError(t, err)
	require.Equal(t, "71", gitlab.ProviderID)
	require.Equal(t, core.GitHubPrincipalUser, gitlab.ProviderType)
	for _, expected := range []*core.MavenDomainHealth{nil, {ProviderID: "41", ProviderType: "organization"}, github} {
		domain := &core.MavenDomain{VerificationType: core.MavenVerificationGitHub, VerificationHost: "example", Verified: true, Health: expected}
		health, err := inspectDomainHealth(context.Background(), client, domain, "")
		require.NoError(t, err)
		want := "prohibited"
		if expected == nil {
			want = "hold"
		} else if expected == github {
			want = "active"
		}
		require.Equal(t, want, health.Status)
	}
}

func TestRDAPDomainHealth(t *testing.T) {
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for status, want := range map[string]string{
		"active": "active", "serverHold": "hold", "client hold": "hold", "pendingDelete": "expired",
		"restorable": "expired", "redemption period": "expired", "pending restore": "expired",
		"clientRenewProhibited": "prohibited", "server renew prohibited": "prohibited", "renew prohibited": "prohibited",
	} {
		health, err := rdapDomainHealth(rdapDomain{ObjectClassName: "domain", LDHName: "EXAMPLE.COM", Status: []string{status}}, "example.com", now)
		require.NoError(t, err)
		require.Equal(t, want, health.Status, status)
	}
	document := rdapDomain{ObjectClassName: "domain", LDHName: "example.com"}
	document.Events = append(document.Events, struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	}{Action: "expiration", Date: now.Format(time.RFC3339)})
	health, err := rdapDomainHealth(document, "example.com", now)
	require.NoError(t, err)
	require.Equal(t, "expired", health.Status)
	_, err = rdapDomainHealth(document, "other.com", now)
	require.Error(t, err)
	document.Events[0].Date = "invalid"
	_, err = rdapDomainHealth(document, "example.com", now)
	require.Error(t, err)
}

func TestVerificationResponseRejectsChunkedOversizeBody(t *testing.T) {
	response := verificationResponse(http.StatusOK, `{"bio":"`+strings.Repeat("x", verificationBodySize)+`"}`)
	response.ContentLength = -1
	var destination struct {
		Bio string `json:"bio"`
	}
	err := readVerificationResponse(response, &destination)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size limit")
}
