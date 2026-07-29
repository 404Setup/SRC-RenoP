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
	"runtime"
	"time"

	"github.com/gofiber/fiber/v3"

	"renop/core"
	"renop/pb"
	"renop/updater"
	"renop/utils/protohttp"
	"renop/version"
)

func SetupRoutes(api fiber.Router, state *core.AppState) {
	api.Get("/status/instance", func(c fiber.Ctx) error { return GetInstanceStatus(c, state) })
	api.Get("/status/snapshots", func(c fiber.Ctx) error { return GetStatusSnapshots(c, state) })
	api.Get("/status/health", GetHealth)
}

func GetInstanceStatus(c fiber.Ctx, state *core.AppState) error {
	memPtr := CachedMemory.Load()
	var usedMemory, totalMemory, renopUsedDisk, diskUsed, diskTotal uint64
	var logicalCores, physicalCores int
	if memPtr != nil {
		usedMemory = memPtr.UsedMemory
		totalMemory = memPtr.TotalMemory
		renopUsedDisk = memPtr.RenopUsedDisk
		diskUsed = memPtr.DiskUsed
		diskTotal = memPtr.DiskTotal
		logicalCores = memPtr.LogicalCores
		physicalCores = memPtr.PhysicalCores
	} else {
		logicalCores = cachedLogicalCores
		physicalCores = cachedPhysicalCores
		totalMemory = cachedTotalMemory
		usedMemory = processRSSMiB()
		if usedMemory == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.Sys > m.HeapReleased {
				usedMemory = (m.Sys - m.HeapReleased) / (1024 * 1024)
			} else {
				usedMemory = m.Alloc / (1024 * 1024)
			}
		}
		if totalMemory == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			totalMemory = m.Sys / (1024 * 1024)
		}
		renopUsedDisk, diskUsed, diskTotal = UpdateDiskStats(state)
	}

	totalThreads := uint64(10000)
	usedThreads := uint64(runtime.NumGoroutine())
	availableThreads := totalThreads
	if usedThreads < availableThreads {
		availableThreads -= usedThreads
	} else {
		availableThreads = 0
	}
	failuresCount := state.Inner.FailuresCount.Load()

	uptime := time.Now().UnixMilli() - state.Inner.StartTime

	res := &pb.InstanceStatus{
		Version:          version.Version,
		Development:      version.IsDevelopment(),
		Uptime:           uptime,
		UsedMemory:       usedMemory,
		TotalMemory:      totalMemory,
		RenopUsedDisk:    renopUsedDisk,
		DiskUsed:         diskUsed,
		DiskTotal:        diskTotal,
		UsedThreads:      usedThreads,
		AvailableThreads: availableThreads,
		TotalThreads:     totalThreads,
		Architecture:     runtime.GOARCH,
		Os:               runtime.GOOS,
		LogicalCores:     int32(logicalCores),
		PhysicalCores:    int32(physicalCores),
		FailuresCount:    failuresCount,
		UpdateState:      updater.ToPbUpdateState(updater.GetUpdateState()),
	}

	return protohttp.Write(c, res)
}

func GetStatusSnapshots(c fiber.Ctx, state *core.AppState) error {
	ptr := state.Inner.StatusSnapshots.Load()
	if ptr == nil {
		return protohttp.Write(c, &pb.StatusSnapshotList{Snapshots: []*pb.StatusSnapshot{}})
	}
	return protohttp.Write(c, pb.FromStatusSnapshots(*ptr))
}

func GetHealth(c fiber.Ctx) error {
	return c.JSON("UP")
}
