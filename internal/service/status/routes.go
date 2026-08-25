/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package status collects and exposes process and storage health metrics.
package status

import (
	"fmt"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/klauspost/cpuid/v2"

	"renop/internal/core"
	"renop/internal/service/updater"
	"renop/internal/utils/protohttp"
	"renop/internal/version"
	"renop/pkg/pb"
)

var archString = getArchitectureString()

func SetupRoutes(api fiber.Router, state *core.AppState) {
	api.Get("/status/instance", func(c fiber.Ctx) error { return GetInstanceStatus(c, state) })
	api.Get("/status/snapshots", func(c fiber.Ctx) error { return GetStatusSnapshots(c, state) })
	api.Get("/status/health", GetHealth)
}

func getArchitectureString() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		lvl := min(max(cpuid.CPU.X64Level(), 1), 4)
		return fmt.Sprintf("amd64 (v%d)", lvl)
	}
	return arch
}

func GetInstanceStatus(c fiber.Ctx, state *core.AppState) error {
	memPtr := CachedMemory.Load()
	var usedMemory, vssMemory, totalMemory, renopUsedDisk, diskUsed, diskTotal uint64
	var logicalCores, physicalCores int
	if memPtr != nil {
		usedMemory = memPtr.UsedMemory
		vssMemory = memPtr.VssMemory
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
		usedMemory, vssMemory = processMemoryWithFallback()
		if totalMemory == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			totalMemory = m.Sys
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
		VssMemory:        vssMemory,
		TotalMemory:      totalMemory,
		RenopUsedDisk:    renopUsedDisk,
		DiskUsed:         diskUsed,
		DiskTotal:        diskTotal,
		UsedThreads:      usedThreads,
		AvailableThreads: availableThreads,
		TotalThreads:     totalThreads,
		Architecture:     archString,
		Os:               runtime.GOOS,
		LogicalCores:     int32(logicalCores),
		PhysicalCores:    int32(physicalCores),
		FailuresCount:    failuresCount,
		UpdateState:      updater.ToPbUpdateState(updater.GetUpdateState()),
		DebugMode:        DebugModeActive(),
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
