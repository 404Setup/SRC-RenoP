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
	"net/http"
	"testing"
)

func TestCheckMirrorRedirect(t *testing.T) {
	original, _ := http.NewRequest(http.MethodGet, "https://repo.example/artifact.jar", nil)
	original.Header.Set("Authorization", "Bearer secret")

	sameOrigin, _ := http.NewRequest(http.MethodGet, "https://repo.example:443/moved.jar", nil)
	if err := checkMirrorRedirect(sameOrigin, []*http.Request{original}); err != nil {
		t.Fatalf("same-origin redirect rejected: %v", err)
	}

	crossOrigin, _ := http.NewRequest(http.MethodGet, "https://cdn.example/artifact.jar", nil)
	if err := checkMirrorRedirect(crossOrigin, []*http.Request{original}); err == nil {
		t.Fatal("credentialed cross-origin redirect was allowed")
	}

	downgrade, _ := http.NewRequest(http.MethodGet, "http://repo.example/artifact.jar", nil)
	if err := checkMirrorRedirect(downgrade, []*http.Request{original}); err == nil {
		t.Fatal("HTTPS downgrade redirect was allowed")
	}
}
