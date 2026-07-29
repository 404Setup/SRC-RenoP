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
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	"renop/config"
	"renop/core"
	"renop/index"
	"renop/updater"
	"renop/utils"
)

const maxStatusSnapshots = 15

type CachedMemoryState struct {
	UsedMemory    uint64
	VssMemory     uint64
	TotalMemory   uint64
	RenopUsedDisk uint64
	DiskUsed      uint64
	DiskTotal     uint64
	LogicalCores  int
	PhysicalCores int
}

var (
	CachedMemory atomic.Pointer[CachedMemoryState]

	cachedLogicalCores  int
	cachedPhysicalCores int
	cachedTotalMemory   uint64

	cachedDiskTotal     uint64
	cachedFixedSize     uint64
	fixedSizesOnce      sync.Once
	cachedStorageSize   uint64
	storageDirty        atomic.Bool
	lastDiskUpdate      time.Time
	lastDiskUpdateMutex sync.Mutex
	cachedDiskUsed      uint64
)

func init() {
	storageDirty.Store(true)
	index.OnStorageUpdated = MarkStorageUpdated
	updater.CanAllocateDiskSpace = func(req uint64) bool {
		return CanAllocateDiskSpace(nil, req)
	}
	cachedLogicalCores = runtime.NumCPU()
	cachedPhysicalCores = cachedLogicalCores
	if counts, err := cpu.Counts(false); err == nil {
		cachedPhysicalCores = counts
	}
	if v, err := mem.VirtualMemory(); err == nil {
		cachedTotalMemory = v.Total
	}
}

func MarkStorageUpdated() {
	storageDirty.Store(true)
}

// processMemoryWithFallback returns RSS and VSS in bytes, with runtime.MemStats fallback.
func processMemoryWithFallback() (rss, vss uint64) {
	rss, vss = processMemoryBytes()
	if rss == 0 {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		if m.Sys > m.HeapReleased {
			rss = m.Sys - m.HeapReleased
		} else {
			rss = m.Alloc
		}
	}
	if vss == 0 {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		vss = max(m.Sys, rss)
	}
	return rss, vss
}

func GetFreeDiskSpace(state *core.AppState) uint64 {
	storagePath := "storage"
	if state != nil {
		if cfgVal := state.Inner.Config.Load(); cfgVal != nil {
			if c, ok := cfgVal.(*config.Config); ok && c.StoragePath != "" {
				storagePath = c.StoragePath
			}
		}
	}
	targetDir := "."
	if absPath, err := filepath.Abs(storagePath); err == nil {
		if fi, err := os.Stat(absPath); err == nil && fi.IsDir() {
			targetDir = absPath
		}
	}
	if usage, err := disk.Usage(targetDir); err == nil && usage.Total > 0 {
		return usage.Free
	}
	if usage, err := disk.Usage("."); err == nil && usage.Total > 0 {
		return usage.Free
	}
	return math.MaxUint64
}

func CanAllocateDiskSpace(state *core.AppState, requiredBytes uint64) bool {
	free := GetFreeDiskSpace(state)
	return free > requiredBytes
}

func UpdateDiskStats(state *core.AppState) (renopUsed, diskUsed, diskTotal uint64) {
	lastDiskUpdateMutex.Lock()
	defer lastDiskUpdateMutex.Unlock()

	storagePath := "storage"
	if state != nil {
		if cfgVal := state.Inner.Config.Load(); cfgVal != nil {
			if c, ok := cfgVal.(*config.Config); ok && c.StoragePath != "" {
				storagePath = c.StoragePath
			}
		}
	}

	targetDir := "."
	if absPath, err := filepath.Abs(storagePath); err == nil {
		targetDir = absPath
	}

	if cachedDiskTotal == 0 {
		if usage, err := disk.Usage(targetDir); err == nil && usage.Total > 0 {
			cachedDiskTotal = usage.Total
		} else if usage, err := disk.Usage("."); err == nil && usage.Total > 0 {
			cachedDiskTotal = usage.Total
		}
	}

	fixedSizesOnce.Do(func() {
		var fixedBytes uint64
		if exePath, err := os.Executable(); err == nil {
			if fi, err := os.Stat(exePath); err == nil {
				fixedBytes += uint64(fi.Size())
			}
		}
		files := []string{"config.yaml", "repositories.yaml", "tokens.yaml", "index.json", "sessions.bin", "sessions.json"}
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
				fixedBytes += uint64(fi.Size())
			}
		}
		cachedFixedSize = fixedBytes
	})

	now := time.Now()
	if !lastDiskUpdate.IsZero() && now.Sub(lastDiskUpdate) < 5*time.Minute {
		renopUsed = cachedFixedSize + cachedStorageSize
		diskUsed = cachedDiskUsed
		diskTotal = cachedDiskTotal
		return
	}

	if storageDirty.Swap(false) || cachedStorageSize == 0 {
		var totalStorageBytes uint64
		if fi, err := os.Stat(storagePath); err == nil {
			if fi.IsDir() {
				_ = filepath.WalkDir(storagePath, func(p string, d os.DirEntry, err error) error {
					if err == nil && !d.IsDir() {
						if info, err := d.Info(); err == nil {
							totalStorageBytes += uint64(info.Size())
						}
					}
					return nil
				})
			} else {
				totalStorageBytes = uint64(fi.Size())
			}
		}
		cachedStorageSize = totalStorageBytes
	}

	if usage, err := disk.Usage(targetDir); err == nil && usage.Total > 0 {
		cachedDiskUsed = usage.Used
		if cachedDiskTotal == 0 {
			cachedDiskTotal = usage.Total
		}
	} else if usage, err := disk.Usage("."); err == nil {
		cachedDiskUsed = usage.Used
		if cachedDiskTotal == 0 {
			cachedDiskTotal = usage.Total
		}
	}

	lastDiskUpdate = now

	renopUsed = cachedFixedSize + cachedStorageSize
	diskUsed = cachedDiskUsed
	diskTotal = cachedDiskTotal
	return
}

func StartStatusSnapshotScheduler(state *core.AppState, intervalDuration time.Duration) {
	go func() {
		ticker := time.NewTicker(intervalDuration)
		defer ticker.Stop()

		updateMemory := func() {
			usedMemory, vssMemory := processMemoryWithFallback()
			totalMemory := cachedTotalMemory
			if totalMemory == 0 {
				if v, err := mem.VirtualMemory(); err == nil {
					totalMemory = v.Total
					cachedTotalMemory = totalMemory
				}
			}
			logicalCores := cachedLogicalCores
			physicalCores := cachedPhysicalCores

			renopUsedDisk, diskUsed, diskTotal := UpdateDiskStats(state)

			CachedMemory.Store(&CachedMemoryState{
				UsedMemory:    usedMemory,
				VssMemory:     vssMemory,
				TotalMemory:   totalMemory,
				RenopUsedDisk: renopUsedDisk,
				DiskUsed:      diskUsed,
				DiskTotal:     diskTotal,
				LogicalCores:  logicalCores,
				PhysicalCores: physicalCores,
			})

			usedThreads := uint64(runtime.NumGoroutine())
			snapshot := core.StatusSnapshot{
				Timestamp:   time.Now().UnixMilli(),
				UsedMemory:  usedMemory,
				VssMemory:   vssMemory,
				UsedThreads: usedThreads,
				OpenFiles:   0,
			}

			utils.ReleaseMemoryToOS()

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
					break
				}
			}
		}

		updateMemory()

		for range ticker.C {
			updateMemory()
		}
	}()
}
