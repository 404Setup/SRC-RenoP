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

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

var (
	selfProcess     *process.Process
	selfProcessOnce sync.Once
)

func processMemoryBytes() (rss, vss uint64) {
	if r, v, ok := processMemoryFromProcSelf(); ok {
		return r, v
	}

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

func processMemoryFromProcSelf() (rss, vss uint64, ok bool) {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0, false
	}
	rss = parseProcKBField(status, "VmRSS")
	swap := parseProcKBField(status, "VmSwap")

	var private uint64
	if rollup, err := os.ReadFile("/proc/self/smaps_rollup"); err == nil {
		private = parseProcKBField(rollup, "Private_Dirty") +
			parseProcKBField(rollup, "Private_Clean") +
			parseProcKBField(rollup, "Swap")
	}
	if private == 0 {
		private = rss + swap
	}

	if rss == 0 && private == 0 {
		return 0, 0, false
	}
	vss = privateOrRSS(rss, private)
	return rss, vss, true
}

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
	if m.Sys > m.HeapReleased {
		vss = m.Sys - m.HeapReleased
	} else {
		vss = m.Sys
	}
	if vss < rss {
		vss = rss
	}
	return rss, vss
}

func goRuntimeRetainedBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Sys > m.HeapReleased {
		return m.Sys - m.HeapReleased
	}
	return m.Sys
}

func parseProcKBField(data []byte, key string) uint64 {
	prefix := append([]byte(key), ':')
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line = data[:i]
			data = data[i+1:]
		} else {
			line = data
			data = nil
		}
		if !bytes.HasPrefix(line, prefix) {
			continue
		}
		rest := bytes.TrimSpace(line[len(prefix):])
		if i := bytes.IndexByte(rest, ' '); i >= 0 {
			rest = rest[:i]
		}
		n, err := strconv.ParseUint(string(rest), 10, 64)
		if err != nil {
			return 0
		}
		return n * 1024
	}
	return 0
}

func sanitizeVirtualSize(rss, vms uint64) uint64 {
	if vms == 0 {
		return 0
	}
	if rss == 0 {
		return vms
	}
	const inflateSlack = 256 << 20
	if vms > rss+inflateSlack && vms >= rss*4 {
		return 0
	}
	if vms < rss {
		return rss
	}
	return vms
}
