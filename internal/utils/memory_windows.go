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
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var (
	modKernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procSetProcessWorkingSetSize = modKernel32.NewProc("SetProcessWorkingSetSize")
	networkWorkingSetTrimOnce    sync.Once
	workingSetTrimDelay          = 5 * time.Second
	workingSetTrim               = trimProcessWorkingSet
)

// InitLinuxMemoryTuning is a no-op on Windows.
func InitLinuxMemoryTuning() {}

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
