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
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduleNetworkWorkingSetTrimRunsOnce(t *testing.T) {
	originalDelay := workingSetTrimDelay
	originalTrim := workingSetTrim
	t.Cleanup(func() {
		networkWorkingSetTrimOnce = sync.Once{}
		workingSetTrimDelay = originalDelay
		workingSetTrim = originalTrim
	})

	networkWorkingSetTrimOnce = sync.Once{}
	workingSetTrimDelay = time.Millisecond
	var calls atomic.Int32
	called := make(chan struct{})
	workingSetTrim = func() error {
		if calls.Add(1) == 1 {
			close(called)
		}
		return nil
	}

	for range 10 {
		ScheduleNetworkWorkingSetTrim()
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("scheduled working-set trim did not run")
	}
	time.Sleep(10 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("working-set trim calls = %d, want 1", got)
	}
}
