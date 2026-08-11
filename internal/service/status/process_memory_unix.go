//go:build !windows

/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package status

import "runtime"

func privateOrRSS(rss, private uint64) uint64 {
	if private < rss {
		return rss
	}
	return private
}

func goMemStatsFallback() (rss, vss uint64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rss = m.HeapInuse
	vss = goRuntimeRetainedFromMemStats(&m)
	if vss < rss {
		vss = rss
	}
	return rss, vss
}

func goRuntimeRetainedBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return goRuntimeRetainedFromMemStats(&m)
}

func goRuntimeRetainedFromMemStats(m *runtime.MemStats) uint64 {
	if m.Sys > m.HeapReleased {
		return m.Sys - m.HeapReleased
	}
	return m.Sys
}

func sanitizeVirtualSize(rss, vms uint64) uint64 {
	if vms == 0 {
		return 0
	}
	if rss == 0 {
		return vms
	}
	const inflateSlack = 256 << 20
	if vms > rss && vms-rss > inflateSlack && vms/rss >= 4 {
		return 0
	}
	if vms < rss {
		return rss
	}
	return vms
}
