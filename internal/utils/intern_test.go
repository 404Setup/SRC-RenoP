/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import (
	"testing"
	"unsafe"
)

func TestIntern(t *testing.T) {
	s1 := "test-string-1"
	bytes := []byte(s1)
	s2 := string(bytes)

	ptr1 := unsafe.StringData(s1)
	ptr2 := unsafe.StringData(s2)
	if ptr1 == ptr2 {
		t.Fatalf("test setup error: s1 and s2 should have different data pointers")
	}

	interned1 := Intern(s1)
	interned2 := Intern(s2)

	if interned1 != interned2 {
		t.Errorf("expected same string content, got %q and %q", interned1, interned2)
	}

	ptrI1 := unsafe.StringData(interned1)
	ptrI2 := unsafe.StringData(interned2)

	if ptrI1 != ptrI2 {
		t.Errorf("expected interned strings to share the exact same backing pointer, but got %p and %p", ptrI1, ptrI2)
	}

	if Intern("") != "" {
		t.Errorf("expected empty string from Intern(\"\")")
	}
}
