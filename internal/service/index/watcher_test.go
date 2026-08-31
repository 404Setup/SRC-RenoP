/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package index

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDirectoryScanWorkerCoalescesOverflow(t *testing.T) {
	requests := make(chan string, 1)
	var scanAll atomic.Bool
	done := make(chan struct{})
	defer func() {
		select {
		case <-done:
		default:
			close(done)
		}
	}()
	scanned := make(chan string, 1)
	exited := make(chan struct{})
	queueDirectoryScan(requests, &scanAll, "first")
	queueDirectoryScan(requests, &scanAll, "second")
	go func() {
		runDirectoryScanWorker("root", requests, &scanAll, done, func(path string) {
			scanned <- path
		})
		close(exited)
	}()
	select {
	case path := <-scanned:
		if path != "root" {
			t.Fatalf("overflow scan path = %q, want root", path)
		}
	case <-time.After(time.Second):
		t.Fatal("directory scan worker did not run")
	}
	close(done)
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("directory scan worker did not stop")
	}
}
