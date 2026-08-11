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

package status

import (
	"math"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const procSelfStatm = "/proc/self/statm"

var (
	selfStatmReader = procStatmReader{fd: -1}
	processPageSize = uint64(os.Getpagesize())
)

type procStatmReader struct {
	mu sync.Mutex
	fd int
}

func processMemoryBytes() (rss, vss uint64) {
	if rss, vss, ok := selfStatmReader.sample(); ok {
		return rss, vss
	}
	return goMemStatsFallback()
}

func (r *procStatmReader) sample() (rss, vss uint64, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for range 2 {
		if r.fd < 0 {
			fd, err := unix.Open(procSelfStatm, unix.O_RDONLY|unix.O_CLOEXEC, 0)
			if err != nil {
				return 0, 0, false
			}
			r.fd = fd
		}
		if _, err := unix.Seek(r.fd, 0, 0); err != nil {
			r.reset()
			continue
		}

		var buf [128]byte
		n, err := unix.Read(r.fd, buf[:])
		if err == nil {
			if rss, vss, ok = parseProcStatm(buf[:n], processPageSize); ok {
				return rss, vss, true
			}
		}
		r.reset()
	}
	return 0, 0, false
}

func (r *procStatmReader) reset() {
	if r.fd >= 0 {
		_ = unix.Close(r.fd)
		r.fd = -1
	}
}

func parseProcStatm(data []byte, pageSize uint64) (rss, vss uint64, ok bool) {
	vmsPages, rest, ok := nextProcStatmUint(data)
	if !ok {
		return 0, 0, false
	}
	rssPages, _, ok := nextProcStatmUint(rest)
	if !ok || pageSize == 0 || vmsPages > math.MaxUint64/pageSize || rssPages > math.MaxUint64/pageSize {
		return 0, 0, false
	}

	rss = rssPages * pageSize
	vss = vmsPages * pageSize
	if rss == 0 && vss == 0 {
		return 0, 0, false
	}
	vss = privateOrRSS(rss, sanitizeVirtualSize(rss, vss))
	return rss, vss, true
}

func nextProcStatmUint(data []byte) (value uint64, rest []byte, ok bool) {
	i := 0
	for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r' || data[i] == '\n') {
		i++
	}
	start := i
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		digit := uint64(data[i] - '0')
		if value > (math.MaxUint64-digit)/10 {
			return 0, nil, false
		}
		value = value*10 + digit
		i++
	}
	if i == start || (i < len(data) && data[i] != ' ' && data[i] != '\t' && data[i] != '\r' && data[i] != '\n') {
		return 0, nil, false
	}
	return value, data[i:], true
}
