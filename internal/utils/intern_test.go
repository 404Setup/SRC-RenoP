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
	"reflect"
	"testing"
	"unsafe"
)

func TestIntern(t *testing.T) {
	s1 := "test-string-1"
	bytes := []byte(s1)
	s2 := string(bytes)

	hdr1 := (*reflect.StringHeader)(unsafe.Pointer(&s1))
	hdr2 := (*reflect.StringHeader)(unsafe.Pointer(&s2))
	if hdr1.Data == hdr2.Data {
		t.Fatalf("test setup error: s1 and s2 should have different data pointers")
	}

	interned1 := Intern(s1)
	interned2 := Intern(s2)

	if interned1 != interned2 {
		t.Errorf("expected same string content, got %q and %q", interned1, interned2)
	}

	hdrI1 := (*reflect.StringHeader)(unsafe.Pointer(&interned1))
	hdrI2 := (*reflect.StringHeader)(unsafe.Pointer(&interned2))

	if hdrI1.Data != hdrI2.Data {
		t.Errorf("expected interned strings to share the exact same backing pointer, but got %x and %x", hdrI1.Data, hdrI2.Data)
	}

	if Intern("") != "" {
		t.Errorf("expected empty string from Intern(\"\")")
	}
}
