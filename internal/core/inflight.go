/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"sync"
	"time"
)

type InFlightDownload struct {
	Done    chan struct{}
	Success bool
}

type InFlightManager struct {
	mu sync.Mutex
	m  map[string]*InFlightDownload
}

func NewInFlightManager() *InFlightManager {
	return &InFlightManager{
		m: make(map[string]*InFlightDownload),
	}
}

func (mgr *InFlightManager) LockPath(path string) (*InFlightDownload, bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if dl, ok := mgr.m[path]; ok {
		return dl, true
	}

	dl := &InFlightDownload{
		Done:    make(chan struct{}),
		Success: false,
	}
	mgr.m[path] = dl
	return dl, false
}

// AcquirePath repeatedly attempts to lock path, waiting for any in-flight holder
// to finish before acquiring the lock for the caller.
func (mgr *InFlightManager) AcquirePath(path string) *InFlightDownload {
	for {
		dl, loaded := mgr.LockPath(path)
		if !loaded {
			return dl
		}
		mgr.Wait(dl)
	}
}

func (mgr *InFlightManager) UnlockPath(path string, dl *InFlightDownload, success bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if current, ok := mgr.m[path]; ok && current == dl {
		dl.Success = success
		close(dl.Done)
		delete(mgr.m, path)
	}
}

func (mgr *InFlightManager) Wait(dl *InFlightDownload) {
	<-dl.Done
}

func (mgr *InFlightManager) WaitTimeout(dl *InFlightDownload, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-dl.Done:
		return true
	case <-timer.C:
		return false
	}
}
