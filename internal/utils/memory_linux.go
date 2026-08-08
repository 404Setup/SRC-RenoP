//go:build linux

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
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

var linuxMemoryOnce sync.Once

func InitLinuxMemoryTuning() {
	linuxMemoryOnce.Do(func() {
		if os.Getenv("GOGC") == "" {
			debug.SetGCPercent(35)
		}
		if os.Getenv("GOMEMLIMIT") != "" {
			return
		}
		if limit, ok := linuxSoftMemoryLimit(); ok {
			debug.SetMemoryLimit(limit)
		}
	})
}

// ScheduleNetworkWorkingSetTrim is only needed on Windows.
func ScheduleNetworkWorkingSetTrim() {}

func linuxSoftMemoryLimit() (int64, bool) {
	if n, ok := readCgroupMemoryMax(); ok && n > 0 {
		limit := n * 70 / 100
		const minLimit = 64 << 20
		const maxLimit = 4 << 30
		if limit < minLimit {
			return 0, false
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		return limit, true
	}
	if avail, ok := readMemAvailable(); ok && avail > 256<<20 {
		limit := avail / 2
		if limit > 1<<30 {
			limit = 1 << 30
		}
		if limit < 64<<20 {
			return 0, false
		}
		return limit, true
	}
	return 0, false
}

func readCgroupMemoryMax() (int64, bool) {
	if b, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(b))
		if s != "" && s != "max" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
				return n, true
			}
		}
	}
	if b, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		s := strings.TrimSpace(string(b))
		if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
			if n > 1<<62 {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

func readMemAvailable() (int64, bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
