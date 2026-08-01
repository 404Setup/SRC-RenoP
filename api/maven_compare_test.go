/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"strings"
	"testing"
)

func isDigit(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func CompareVersionsOriginal(a, b string) int {
	aParts := strings.FieldsFunc(a, func(r rune) bool {
		return r == '-' || r == '.' || r == '_'
	})
	bParts := strings.FieldsFunc(b, func(r rune) bool {
		return r == '-' || r == '.' || r == '_'
	})

	aLen := len(aParts)
	bLen := len(bParts)

	maxLen := max(bLen, aLen)

	for i := range maxLen {
		aFrag := "0"
		if i < aLen {
			aFrag = aParts[i]
		}
		bFrag := "0"
		if i < bLen {
			bFrag = bParts[i]
		}

		aIsDigit := isDigit(aFrag)
		bIsDigit := isDigit(bFrag)

		if aIsDigit && bIsDigit {
			aStripped := strings.TrimLeft(aFrag, "0")
			bStripped := strings.TrimLeft(bFrag, "0")

			if len(aStripped) != len(bStripped) {
				if len(aStripped) < len(bStripped) {
					return -1
				}
				return 1
			}
			if aStripped != bStripped {
				if aStripped < bStripped {
					return -1
				}
				return 1
			}
		} else if aIsDigit || bIsDigit {
			if aIsDigit {
				return 1
			}
			return -1
		}

		if aFrag != bFrag {
			if aFrag < bFrag {
				return -1
			}
			return 1
		}
	}

	if aLen < bLen {
		return -1
	} else if aLen > bLen {
		return 1
	}
	return 0
}

func CompareVersionsNew(a, b string) int {
	aIdx, bIdx := 0, 0
	aLen, bLen := len(a), len(b)

	isSep := func(c byte) bool { return c == '-' || c == '.' || c == '_' }

	aPartsCount, bPartsCount := 0, 0
	for i := range aLen {
		if !isSep(a[i]) && (i == 0 || isSep(a[i-1])) {
			aPartsCount++
		}
	}
	for i := range bLen {
		if !isSep(b[i]) && (i == 0 || isSep(b[i-1])) {
			bPartsCount++
		}
	}

	for aIdx < aLen || bIdx < bLen {
		for aIdx < aLen && isSep(a[aIdx]) {
			aIdx++
		}
		for bIdx < bLen && isSep(b[bIdx]) {
			bIdx++
		}

		if aIdx == aLen && bIdx == bLen {
			break
		}

		aFrag := "0"
		bFrag := "0"

		if aIdx < aLen {
			start := aIdx
			for aIdx < aLen && !isSep(a[aIdx]) {
				aIdx++
			}
			aFrag = a[start:aIdx]
		}

		if bIdx < bLen {
			start := bIdx
			for bIdx < bLen && !isSep(b[bIdx]) {
				bIdx++
			}
			bFrag = b[start:bIdx]
		}

		aIsDigit := true
		bIsDigit := true

		if aFrag == "" {
			aIsDigit = false
		} else {
			for i := 0; i < len(aFrag); i++ {
				if aFrag[i] < '0' || aFrag[i] > '9' {
					aIsDigit = false
					break
				}
			}
		}

		if bFrag == "" {
			bIsDigit = false
		} else {
			for i := 0; i < len(bFrag); i++ {
				if bFrag[i] < '0' || bFrag[i] > '9' {
					bIsDigit = false
					break
				}
			}
		}

		if aIsDigit && bIsDigit {
			aStrippedStart := 0
			for aStrippedStart < len(aFrag) && aFrag[aStrippedStart] == '0' {
				aStrippedStart++
			}
			aStripped := aFrag[aStrippedStart:]

			bStrippedStart := 0
			for bStrippedStart < len(bFrag) && bFrag[bStrippedStart] == '0' {
				bStrippedStart++
			}
			bStripped := bFrag[bStrippedStart:]

			if len(aStripped) != len(bStripped) {
				if len(aStripped) < len(bStripped) {
					return -1
				}
				return 1
			}
			if aStripped != bStripped {
				if aStripped < bStripped {
					return -1
				}
				return 1
			}
		} else if aIsDigit || bIsDigit {
			if aIsDigit {
				return 1
			}
			return -1
		}

		if aFrag != bFrag {
			if aFrag < bFrag {
				return -1
			}
			return 1
		}
	}

	if aPartsCount < bPartsCount {
		return -1
	} else if aPartsCount > bPartsCount {
		return 1
	}
	return 0
}

func TestEquivalence(t *testing.T) {
	cases := []struct{ a, b string }{
		{a: "1.0.0", b: "1.0.0"},
		{a: "1.0.0", b: "1.0.1"},
		{a: "1.0.1", b: "1.0.0"},
		{a: "1.0", b: "1.0.0"},
		{a: "1.0.0", b: "1.0"},
		{a: "1.0-alpha", b: "1.0-beta"},
		{a: "1.0_alpha", b: "1.0.alpha"},
		{a: "-1.2..3_", b: "1.2.3"},
		{a: "1.0.0-RC1", b: "1.0.0-RC2"},
		{a: "1.0.0-SNAPSHOT", b: "1.0.0-SNAPSHOT"},
		{a: "2.0", b: "1.9.9"},
		{a: "1", b: "1.0"},
		{a: "1-", b: "1"},
		{a: "1.0", b: "1-"},
		{a: "1.0", b: "1..0"},
		{a: "1.0", b: "1.0."},
		{a: "007", b: "7"},
		{a: "7", b: "007"},
		{a: "a0", b: "a0"},
	}

	for _, c := range cases {
		orig := CompareVersionsOriginal(c.a, c.b)
		newR := CompareVersionsNew(c.a, c.b)
		if orig != newR {
			t.Errorf("Mismatch for %q and %q: orig=%d, new=%d", c.a, c.b, orig, newR)
		}
	}
}

func BenchmarkCompareVersionsOriginal(b *testing.B) {
	v1 := "1.2.3-alpha.4_5"
	v2 := "1.2.3-beta.1_0"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareVersionsOriginal(v1, v2)
	}
}

func BenchmarkCompareVersionsNew(b *testing.B) {
	v1 := "1.2.3-alpha.4_5"
	v2 := "1.2.3-beta.1_0"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompareVersionsNew(v1, v2)
	}
}
