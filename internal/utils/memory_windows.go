//go:build windows

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
	"errors"
	"log"
	"os"
	"runtime/debug"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetProcessWorkingSetSize = modKernel32.NewProc("SetProcessWorkingSetSize")
	procGlobalMemoryStatusEx     = modKernel32.NewProc("GlobalMemoryStatusEx")
	networkWorkingSetTrimOnce    sync.Once
	workingSetTrimDelay          = 5 * time.Second
	workingSetTrim               = trimProcessWorkingSet
	windowsMemoryOnce            sync.Once
)

type memoryStatusEx struct {
	cbSize                  uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

// InitMemoryTuning configures GC pacing and soft memory limits on Windows.
func InitMemoryTuning() {
	windowsMemoryOnce.Do(func() {
		if os.Getenv("GOGC") == "" {
			debug.SetGCPercent(35)
		}
		if os.Getenv("GOMEMLIMIT") != "" {
			return
		}
		if limit, ok := windowsSoftMemoryLimit(); ok {
			debug.SetMemoryLimit(limit)
		}
	})
}

// InitLinuxMemoryTuning is retained for backward compatibility.
func InitLinuxMemoryTuning() {
	InitMemoryTuning()
}

func windowsSoftMemoryLimit() (int64, bool) {
	var memStatus memoryStatusEx
	memStatus.cbSize = uint32(unsafe.Sizeof(memStatus))
	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if r1 == 0 {
		return 0, false
	}
	if memStatus.ullAvailPhys > 256<<20 {
		limit := int64(memStatus.ullAvailPhys / 2)
		if limit > 2<<30 {
			limit = 2 << 30
		}
		if limit < 64<<20 {
			return 0, false
		}
		return limit, true
	}
	return 0, false
}

// ScheduleNetworkWorkingSetTrim schedules one delayed trim after the process
// first uses an outbound network client. Some Windows endpoint-security tools
// fault the complete Go executable image into RAM when networking starts. Go
// 1.26 images include a 32 MiB dormant FIPS entropy buffer, so those clean,
// unused pages otherwise remain in the working set indefinitely.
func ScheduleNetworkWorkingSetTrim() {
	networkWorkingSetTrimOnce.Do(func() {
		time.AfterFunc(workingSetTrimDelay, func() {
			if err := workingSetTrim(); err != nil {
				log.Printf("Unable to trim the initial network working set: %v", err)
			}
		})
	})
}

func trimProcessWorkingSet() error {
	result, _, callErr := procSetProcessWorkingSetSize.Call(
		uintptr(windows.CurrentProcess()), ^uintptr(0), ^uintptr(0),
	)
	if result != 0 {
		return nil
	}
	if callErr != nil && !errors.Is(callErr, windows.ERROR_SUCCESS) {
		return callErr
	}
	return errors.New("SetProcessWorkingSetSize returned failure")
}
