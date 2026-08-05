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

func CompareVersions(a, b string) int {
	aIdx, bIdx := 0, 0
	aLen, bLen := len(a), len(b)

	isSep := func(c byte) bool { return c == '-' || c == '.' || c == '_' }

	aPartsCount, bPartsCount := 0, 0

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
			if len(aFrag) > 0 {
				aPartsCount++
			}
		}

		if bIdx < bLen {
			start := bIdx
			for bIdx < bLen && !isSep(b[bIdx]) {
				bIdx++
			}
			bFrag = b[start:bIdx]
			if len(bFrag) > 0 {
				bPartsCount++
			}
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
