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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"renop/internal/service/audit"
)

func TestAuditActionsHaveEveryLocaleTranslation(t *testing.T) {
	actions := audit.KnownActions()
	seen := make(map[string]struct{}, len(actions))
	validAction := regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
	for _, action := range actions {
		if !validAction.MatchString(action) {
			t.Fatalf("invalid audit action identifier %q", action)
		}
		if _, exists := seen[action]; exists {
			t.Fatalf("duplicate audit action identifier %q", action)
		}
		seen[action] = struct{}{}
	}

	localeRoot := filepath.Join("renop-html", "js", "i18n")
	locales, err := os.ReadDir(localeRoot)
	if err != nil {
		t.Fatal(err)
	}
	localeCount := 0
	for _, locale := range locales {
		if !locale.IsDir() {
			continue
		}
		localeCount++
		fragments, err := filepath.Glob(filepath.Join(localeRoot, locale.Name(), "*.js"))
		if err != nil {
			t.Fatal(err)
		}
		var catalog strings.Builder
		for _, fragment := range fragments {
			source, err := os.ReadFile(fragment)
			if err != nil {
				t.Fatal(err)
			}
			catalog.Write(source)
		}
		catalogSource := catalog.String()
		for _, action := range actions {
			key := strconv.Quote("audit.action."+action) + ":"
			if !strings.Contains(catalogSource, key) {
				t.Errorf("locale %s is missing %s", locale.Name(), key)
			}
		}
	}
	if localeCount == 0 {
		t.Fatal("no frontend locale catalogs found")
	}
}
