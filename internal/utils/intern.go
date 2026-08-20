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
	"strings"
	"sync/atomic"

	"github.com/llxisdsh/pb"
)

var (
	stringPool pb.MapOf[string, string]
	poolSize   atomic.Int64
)

const (
	maxInternedStrings = 20_000
	internEvictionBatch = 2_000
)

func Intern(s string) string {
	if s == "" {
		return ""
	}
	if val, ok := stringPool.Load(s); ok {
		return val
	}

	if poolSize.Load() >= maxInternedStrings {
		var evicted int64
		stringPool.Range(func(k string, v string) bool {
			if _, loaded := stringPool.LoadAndDelete(k); loaded {
				evicted++
			}
			return evicted < internEvictionBatch
		})
		if evicted > 0 {
			poolSize.Add(-evicted)
		}
	}

	cloned := strings.Clone(s)
	actual, loaded := stringPool.LoadOrStore(cloned, cloned)
	if !loaded {
		poolSize.Add(1)
	}
	return actual
}
