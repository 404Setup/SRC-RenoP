/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package locale

import (
	"strings"
	"testing"
)

func TestSupportedLanguagesAndHeaderPreference(t *testing.T) {
	for _, code := range Codes {
		if got := Match(strings.ToLower(code)); got != code {
			t.Fatalf("Match(%q) = %q", code, got)
		}
	}
	for input, want := range map[string]string{"fr": "fr-FR", "pt-BR": "pt-PT", "zh-Hant": "zh-TW", "yue-HK": "zh-YUE", "invalid/value": "", "": ""} {
		if got := Match(input); got != want {
			t.Errorf("Match(%q) = %q, want %q", input, got, want)
		}
	}
	if got := FromHeader("en-US;q=0.2, ja-JP;q=0.9"); got != "ja-JP" {
		t.Fatalf("weighted language = %q", got)
	}
	for _, value := range []string{"", "invalid/value", strings.Repeat("fr,", 600)} {
		if got := FromHeader(value); got != Default {
			t.Errorf("invalid header selected %q", got)
		}
	}
}
