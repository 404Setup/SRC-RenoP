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
	"os"
	"runtime/debug"

	"golang.org/x/sys/windows"
)

var (
	modKernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetProcessWorkingSetSize = modKernel32.NewProc("SetProcessWorkingSetSize")
)

// InitLinuxMemoryTuning is a no-op on Windows.
func InitLinuxMemoryTuning() {}

// TrimProcessWorkingSet requests Windows kernel to trim unreferenced physical pages from process Working Set.
func TrimProcessWorkingSet() {
	// This is a very poor approach, intended to solve the persistently difficult-to-solve
	// HTTP client RSS memory leak problem in Windows.
	//
	// I don't know why either, but it's just really hard to fix.
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA, false, uint32(os.Getpid()))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_, _, _ = procSetProcessWorkingSetSize.Call(uintptr(h), ^uintptr(0), ^uintptr(0))
}

// ReleaseMemoryToOS forces Go garbage collection, frees OS memory, and trims Windows working set.
func ReleaseMemoryToOS() {
	debug.FreeOSMemory()
	TrimProcessWorkingSet()
}
