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
	"testing"
	"time"

	"renop/core"
	"renop/index"
)

func TestMaxStatusSnapshotsConstant(t *testing.T) {
	if maxStatusSnapshots != 15 {
		t.Fatalf("expected maxStatusSnapshots=15, got %d", maxStatusSnapshots)
	}
}

// appendStatusSnapshot mirrors the retention logic used by StartStatusSnapshotScheduler.
func appendStatusSnapshot(state *core.AppState, snapshot core.StatusSnapshot) {
	for {
		currentPtr := state.Inner.StatusSnapshots.Load()
		var currentSlice []core.StatusSnapshot
		if currentPtr != nil {
			currentSlice = *currentPtr
		}
		nextSlice := make([]core.StatusSnapshot, 0, len(currentSlice)+1)
		nextSlice = append(nextSlice, currentSlice...)
		nextSlice = append(nextSlice, snapshot)
		if len(nextSlice) > maxStatusSnapshots {
			nextSlice = nextSlice[len(nextSlice)-maxStatusSnapshots:]
		}
		if state.Inner.StatusSnapshots.CompareAndSwap(currentPtr, &nextSlice) {
			return
		}
	}
}

func TestStatusSnapshotsRetainAtMost15(t *testing.T) {
	state := core.NewAppState()
	for i := range 40 {
		appendStatusSnapshot(state, core.StatusSnapshot{
			Timestamp:  int64(i),
			UsedMemory: uint64(i),
		})
	}
	ptr := state.Inner.StatusSnapshots.Load()
	if ptr == nil {
		t.Fatal("expected snapshots to be stored")
	}
	if len(*ptr) != maxStatusSnapshots {
		t.Fatalf("expected %d snapshots, got %d", maxStatusSnapshots, len(*ptr))
	}
	if (*ptr)[0].Timestamp != 25 || (*ptr)[len(*ptr)-1].Timestamp != 39 {
		t.Fatalf("unexpected retention window: first=%d last=%d", (*ptr)[0].Timestamp, (*ptr)[len(*ptr)-1].Timestamp)
	}
}

func TestDiskStats5MinuteCacheAndStorageDirty(t *testing.T) {
	state := core.NewAppState()

	renop1, diskUsed1, diskTotal1 := UpdateDiskStats(state)
	if diskTotal1 == 0 {
		t.Fatalf("expected non-zero diskTotal")
	}

	renop2, diskUsed2, diskTotal2 := UpdateDiskStats(state)
	if renop1 != renop2 || diskUsed1 != diskUsed2 || diskTotal1 != diskTotal2 {
		t.Fatalf("expected cached values to match on consecutive calls")
	}

	MarkStorageUpdated()
	renop3, _, _ := UpdateDiskStats(state)
	if renop3 == 0 {
		t.Fatalf("expected valid renopUsed after MarkStorageUpdated")
	}
}

func TestDiskStatsUsesFileIndexTotals(t *testing.T) {
	state := core.NewAppState()
	idx := index.NewFileIndex()
	idx.InsertFile("storage/releases/a.jar", index.FileInfo{Size: 1000, ModTime: 1})
	idx.InsertFile("storage/releases/b.jar", index.FileInfo{Size: 2345, ModTime: 1})
	state.Inner.FileIndex = idx

	lastDiskUpdateMutex.Lock()
	lastDiskUpdate = time.Time{}
	lastDiskUpdateMutex.Unlock()
	storageDirty.Store(true)

	renopUsed, _, diskTotal := UpdateDiskStats(state)
	if diskTotal == 0 {
		t.Fatal("expected non-zero diskTotal")
	}
	if renopUsed < 3345 {
		t.Fatalf("renopUsed=%d, want at least index total 3345", renopUsed)
	}
}

func TestCanAllocateDiskSpace(t *testing.T) {
	state := core.NewAppState()

	if !CanAllocateDiskSpace(state, 100) {
		t.Logf("CanAllocateDiskSpace returned false for 100 bytes (system might be out of disk space)")
	}

	if CanAllocateDiskSpace(state, math.MaxUint64) {
		t.Fatalf("expected CanAllocateDiskSpace to return false for math.MaxUint64")
	}
}
