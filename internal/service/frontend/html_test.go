/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package frontend

import (
	"strings"
	"testing"

	"renop/internal/config"
)

func TestBundledAssetsEmbedded(t *testing.T) {
	files := []string{
		"index.html",
		"js/main.js",
		"css/style.css",
		"svg/logo.svg",
	}

	for _, file := range files {
		data, err := readAsset(file)
		if err != nil {
			t.Fatalf("expected %s to be embedded, got error: %v", file, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s is empty", file)
		}
	}
}

func TestIndexHtmlUsesBundledAssets(t *testing.T) {
	data, err := readAsset("index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(data)
	for _, needle := range []string{
		`/css/style.css?v={{RENOP.HASH}}`,
		`/js/main.js?v={{RENOP.HASH}}`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("index.html missing bundled asset reference %q", needle)
		}
	}
	if strings.Contains(html, "{{RENOP.CSS_LINKS}}") || strings.Contains(html, "{{RENOP.JS_LINKS}}") {
		t.Fatal("index.html still contains legacy CSS/JS link placeholders")
	}
}

func TestAssetsHashStableAndNonEmpty(t *testing.T) {
	h1 := GetAssetsHash()
	h2 := GetAssetsHash()
	if h1 == "" {
		t.Fatal("assets hash is empty")
	}
	if h1 != h2 {
		t.Fatalf("assets hash not stable: %q vs %q", h1, h2)
	}
}

func TestGenerateIndexHtmlIncludesEscapedLegalNoticeURL(t *testing.T) {
	cfg := config.DefaultFrontendConfig()
	cfg.LegalNoticeUrl = `https://example.com/legal?a=1&b="notice"`

	generated := string(GenerateIndexHtmlFromConfig(&cfg))
	if strings.Contains(generated, "{{RENOP.LEGAL_NOTICE_URL}}") {
		t.Fatal("generated HTML still contains the legal notice placeholder")
	}
	if !strings.Contains(generated, `data-url="https://example.com/legal?a=1&amp;b=&#34;notice&#34;"`) {
		t.Fatal("generated HTML does not contain the escaped legal notice URL")
	}
}
