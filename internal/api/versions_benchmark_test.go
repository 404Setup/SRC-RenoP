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

func BenchmarkExtractLastPartOriginal(b *testing.B) {
	sanitizedGav := "com/example/my-artifact"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parts := strings.Split(sanitizedGav, "/")
		var lastPart string
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" {
				lastPart = parts[i]
				break
			}
		}
		_ = lastPart
	}
}

func BenchmarkExtractLastPartOptimized(b *testing.B) {
	sanitizedGav := "com/example/my-artifact"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var lastPart string
		idx := strings.LastIndexByte(strings.TrimRight(sanitizedGav, "/"), '/')
		if idx != -1 {
			lastPart = sanitizedGav[idx+1:]
		} else {
			lastPart = sanitizedGav
		}
		_ = lastPart
	}
}
