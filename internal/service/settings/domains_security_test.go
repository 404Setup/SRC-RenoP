/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"renop/internal/utils"
)

type externalLinkURLTestCase struct {
	name    string
	url     string
	wantErr bool
}

type publicIPTestCase struct {
	ip   string
	want bool
}

func TestValidateExternalLinkURL(t *testing.T) {
	tests := []externalLinkURLTestCase{
		{name: "HTTPS", url: "https://example.com/legal"},
		{name: "HTTP", url: "http://localhost:8080/legal"},
		{name: "uppercase scheme", url: "HTTPS://example.com/legal"},
		{name: "relative", url: "/legal", wantErr: true},
		{name: "unsupported scheme", url: "javascript:alert(1)", wantErr: true},
		{name: "missing host", url: "https:///legal", wantErr: true},
		{name: "credentials", url: "https://user:secret@example.com/legal", wantErr: true},
		{name: "whitespace", url: "https://example.com/legal notice", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExternalLinkURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateExternalLinkURL(%q) error=%v, wantErr=%v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateFontURL(t *testing.T) {
	tests := []externalLinkURLTestCase{
		{name: "same origin", url: "/fonts/interface.woff2"},
		{name: "HTTPS", url: "https://cdn.example.com/interface.woff2"},
		{name: "HTTP", url: "http://fonts.example.com/interface.woff"},
		{name: "relative without slash", url: "fonts/interface.woff2", wantErr: true},
		{name: "protocol relative", url: "//cdn.example.com/interface.woff2", wantErr: true},
		{name: "credentials", url: "https://user:secret@cdn.example.com/interface.woff2", wantErr: true},
		{name: "unsupported scheme", url: "data:font/woff2;base64,AAAA", wantErr: true},
		{name: "whitespace", url: "https://cdn.example.com/interface font.woff2", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFontURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateFontURL(%q) error=%v, wantErr=%v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestIsValidPublicIP(t *testing.T) {
	tests := []publicIPTestCase{
		{ip: "8.8.8.8", want: true},
		{ip: "1.1.1.1", want: true},
		{ip: "2606:4700:4700::1111", want: true},
		{ip: "127.0.0.1"},
		{ip: "10.0.0.1"},
		{ip: "100.64.0.1"},
		{ip: "169.254.169.254"},
		{ip: "192.0.2.1"},
		{ip: "198.18.0.1"},
		{ip: "224.0.0.1"},
		{ip: "240.0.0.1"},
		{ip: "::1"},
		{ip: "fc00::1"},
		{ip: "fe80::1"},
		{ip: "2001:db8::1"},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := isValidPublicIP(net.ParseIP(tt.ip)); got != tt.want {
				t.Fatalf("isValidPublicIP(%s)=%v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateBackgroundWebP(t *testing.T) {
	signature := []byte("RIFF\x00\x00\x00\x00WEBP")
	valid := append(bytes.Clone(signature), bytes.Repeat([]byte{0x5a}, 20)...)
	if err := validateBackgroundWebP(bytes.NewReader(valid), int64(len(valid))); err != nil {
		t.Fatalf("valid WebP rejected: %v", err)
	}

	invalid := bytes.Clone(valid)
	copy(invalid[8:12], "NOPE")
	if err := validateBackgroundWebP(bytes.NewReader(invalid), int64(len(invalid))); !errors.Is(err, errInvalidBackgroundWebP) {
		t.Fatalf("invalid signature error=%v", err)
	}

	oversized := append(bytes.Clone(signature), bytes.Repeat([]byte{0x7f}, 64)...)
	reader := bytes.NewReader(oversized)
	if err := validateBackgroundWebP(reader, 32); !errors.Is(err, utils.ErrResponseTooLarge) {
		t.Fatalf("oversized body error=%v", err)
	}
	if consumed := len(oversized) - reader.Len(); consumed != 33 {
		t.Fatalf("consumed=%d; validator must stop after maxSize+1 bytes", consumed)
	}
}
