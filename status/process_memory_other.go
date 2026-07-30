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
		ret := goRuntimeRetainedBytes()
		return ret, ret
	}
	mi, err := selfProcess.MemoryInfo()
	if err != nil || mi == nil {
		ret := goRuntimeRetainedBytes()
		return ret, ret
	}
	rss = mi.RSS
	vss = combineVSS(rss, sanitizeVirtualSize(rss, mi.VMS), goRuntimeRetainedBytes())
	return rss, vss
}

func processMemoryFromProcSelf() (rss, vss uint64, ok bool) {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0, false
	}
	rss = parseProcKBField(status, "VmRSS")
	swap := parseProcKBField(status, "VmSwap")

	var privateCommit uint64
	if rollup, err := os.ReadFile("/proc/self/smaps_rollup"); err == nil {
		privateCommit = parseProcKBField(rollup, "Private_Dirty") +
			parseProcKBField(rollup, "Private_Clean") +
			parseProcKBField(rollup, "Swap")
		anon := parseProcKBField(rollup, "Anonymous") + parseProcKBField(rollup, "Swap")
		if anon > privateCommit {
			privateCommit = anon
		}
	}
	if privateCommit == 0 {
		privateCommit = rss + swap
	}

	if rss == 0 && privateCommit == 0 {
		return 0, 0, false
	}

	vss = combineVSS(rss, privateCommit, goRuntimeRetainedBytes())
	return rss, vss, true
}

func goRuntimeRetainedBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Sys > m.HeapReleased {
		return m.Sys - m.HeapReleased
	}
	return m.Sys
}

func combineVSS(rss uint64, parts ...uint64) uint64 {
	v := rss
	for _, p := range parts {
		if p > v {
			v = p
		}
	}
	return v
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
