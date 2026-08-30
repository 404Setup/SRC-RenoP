/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"strings"
	"testing"
)

func TestSafeAuditSessionIDIsStableAndNonAuthenticating(t *testing.T) {
	secret := "session-secret-that-must-not-be-logged"
	id := SafeAuditSessionID(secret)
	if !strings.HasPrefix(id, "sha256:") || len(id) != len("sha256:")+16 {
		t.Fatalf("identifier = %q", id)
	}
	if strings.Contains(id, secret) {
		t.Fatal("audit identifier contains the session secret")
	}
	if got := SafeAuditSessionID(secret); got != id {
		t.Fatalf("identifier is not stable: %q != %q", got, id)
	}
	if got := SafeAuditSessionID(id); got != id {
		t.Fatalf("identifier is not idempotent: %q != %q", got, id)
	}
	if got := SafeAuditSessionID(secret + "-other"); got == id {
		t.Fatal("different session tokens produced the same test identifier")
	}
}
