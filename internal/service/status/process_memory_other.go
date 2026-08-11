//go:build !windows && !linux

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

import (
	"os"
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

var (
	selfProcess     *process.Process
	selfProcessOnce sync.Once
)

func processMemoryBytes() (rss, vss uint64) {
	selfProcessOnce.Do(func() {
		p, err := process.NewProcess(int32(os.Getpid()))
		if err == nil {
			selfProcess = p
		}
	})
	if selfProcess == nil {
		return goMemStatsFallback()
	}
	mi, err := selfProcess.MemoryInfo()
	if err != nil || mi == nil {
		return goMemStatsFallback()
	}
	rss = mi.RSS
	vss = privateOrRSS(rss, sanitizeVirtualSize(rss, mi.VMS))
	return rss, vss
}
