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
			return verificationResponse(http.StatusOK, `{"bio":"unrelated"}`), nil
		case "api.github.com/orgs/example":
			return verificationResponse(http.StatusOK, `{"description":"proof renop-verification=current"}`), nil
		case "gitlab.com/api/v4/groups/example":
			return verificationResponse(http.StatusNotFound, `{}`), nil
		case "gitlab.com/api/v4/users":
			assert.Equal(t, "example", request.URL.Query().Get("username"))
			return verificationResponse(http.StatusOK, `[{"bio":"renop-verification=current"}]`), nil
		default:
			return verificationResponse(http.StatusNotFound, `{}`), nil
		}
	})}
	require.NoError(t, verifyGitHub(context.Background(), client, "example", code))
	require.NoError(t, verifyGitLab(context.Background(), client, "example", code))
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
