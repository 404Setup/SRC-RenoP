/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"renop/internal/config"
)

func BenchmarkProxyAuthOriginal(b *testing.B) {
	mirror := config.Mirror{
		Authorization: &config.MirrorCredentials{
			Method:   "basic",
			Login:    "myuser",
			Password: "mypassword123",
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if mirror.Authorization != nil {
			auth := mirror.Authorization
			method := strings.ToLower(auth.Method)
			if method == "basic" || method == "username/password" {
				credentials := auth.Login + ":" + auth.Password
				encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
				req.Header.Set("Authorization", "Basic "+encoded)
			} else if method == "bearer" || method == "token" {
				token := auth.Password
				if token == "" {
					token = auth.Login
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
	}
}

func BenchmarkProxyAuthOptimized(b *testing.B) {
	mirror := config.Mirror{
		Authorization: &config.MirrorCredentials{
			Method:   "basic",
			Login:    "myuser",
			Password: "mypassword123",
		},
	}
	req, _ := http.NewRequest("GET", "http://example.com", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if mirror.Authorization != nil {
			header := mirror.Authorization.GetAuthHeader()
			if header != "" {
				req.Header.Set("Authorization", header)
			}
		}
	}
}
