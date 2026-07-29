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

package status

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processMemoryCountersEx2 struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
	PrivateWorkingSetSize      uintptr
	SharedCommitUsage          uintptr
}

var (
	modPsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modPsapi.NewProc("GetProcessMemoryInfo")
)

func processMemoryBytes() (rss, vss uint64) {
	h, err := windows.OpenProcess(
		windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ,
		false,
		uint32(os.Getpid()),
	)
	if err != nil {
		h, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
		if err != nil {
			return 0, 0
		}
	}
	defer windows.CloseHandle(h)

	var mem processMemoryCountersEx2
	mem.CB = uint32(unsafe.Sizeof(mem))
	r1, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&mem)),
		uintptr(mem.CB),
	)
	if r1 == 0 {
		return 0, 0
	}

	rss = uint64(mem.PrivateWorkingSetSize)
	if rss == 0 {
		rss = uint64(mem.WorkingSetSize)
	}
	vss = uint64(mem.PagefileUsage)
	if vss == 0 {
		vss = uint64(mem.PrivateUsage)
	}
	if vss == 0 {
		vss = uint64(mem.WorkingSetSize)
	}
	return rss, vss
}
