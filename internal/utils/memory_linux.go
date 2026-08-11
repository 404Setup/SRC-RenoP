//go:build linux

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
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

var linuxMemoryOnce sync.Once

type cgroupMemoryLocation struct {
	groupPath string
	mountRoot string
	mountPath string
	files     []string
}

func InitLinuxMemoryTuning() {
	linuxMemoryOnce.Do(func() {
		if os.Getenv("GOGC") == "" {
			debug.SetGCPercent(35)
		}
		if os.Getenv("GOMEMLIMIT") != "" {
			return
		}
		if limit, ok := linuxSoftMemoryLimit(); ok {
			debug.SetMemoryLimit(limit)
		}
	})
}

// ScheduleNetworkWorkingSetTrim is only needed on Windows.
func ScheduleNetworkWorkingSetTrim() {}

func linuxSoftMemoryLimit() (int64, bool) {
	if n, ok := readCgroupMemoryLimit(); ok && n > 0 {
		// Keep room for goroutine stacks, executable mappings, and native buffers,
		// which are not all covered by Go's runtime memory limit.
		limit := n/100*70 + n%100*70/100
		const minLimit = 64 << 20
		const maxLimit = 4 << 30
		if limit < minLimit {
			return 0, false
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		return limit, true
	}
	if avail, ok := readMemAvailable(); ok && avail > 256<<20 {
		limit := avail / 2
		if limit > 1<<30 {
			limit = 1 << 30
		}
		if limit < 64<<20 {
			return 0, false
		}
		return limit, true
	}
	return 0, false
}

func readCgroupMemoryLimit() (int64, bool) {
	cgroupData, cgroupErr := os.ReadFile("/proc/self/cgroup")
	mountInfoData, mountInfoErr := os.ReadFile("/proc/self/mountinfo")
	if cgroupErr == nil && mountInfoErr == nil {
		if limit, ok := readCgroupMemoryLimitFrom(cgroupData, mountInfoData); ok {
			return limit, true
		}
	}

	// Retain compatibility with minimal containers that do not expose procfs
	// mount metadata but do mount the cgroup hierarchy at the conventional path.
	fallbacks := []cgroupMemoryLocation{
		{groupPath: "/", mountRoot: "/", mountPath: "/sys/fs/cgroup", files: []string{"memory.max", "memory.high"}},
		{groupPath: "/", mountRoot: "/", mountPath: "/sys/fs/cgroup/memory", files: []string{"memory.limit_in_bytes", "memory.soft_limit_in_bytes"}},
	}
	return minimumCgroupLimit(fallbacks)
}

func readCgroupMemoryLimitFrom(cgroupData, mountInfoData []byte) (int64, bool) {
	return minimumCgroupLimit(cgroupMemoryLocations(cgroupData, mountInfoData))
}

func minimumCgroupLimit(locations []cgroupMemoryLocation) (int64, bool) {
	var minimum int64
	for _, location := range locations {
		current, ok := resolveCgroupPath(location)
		if !ok {
			continue
		}
		mountRoot := filepath.Clean(location.mountPath)
		for {
			for _, name := range location.files {
				if limit, ok := readCgroupLimitFile(filepath.Join(current, name)); ok && (minimum == 0 || limit < minimum) {
					minimum = limit
				}
			}
			if current == mountRoot {
				break
			}
			parent := filepath.Dir(current)
			if parent == current || !pathWithin(parent, mountRoot) {
				break
			}
			current = parent
		}
	}
	return minimum, minimum > 0
}

func readCgroupLimitFile(name string) (int64, bool) {
	b, err := os.ReadFile(name)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "" || s == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 1<<62 {
		return 0, false
	}
	return n, true
}

func cgroupMemoryLocations(cgroupData, mountInfoData []byte) []cgroupMemoryLocation {
	var unifiedPath, legacyPath string
	for _, line := range strings.Split(string(cgroupData), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		if parts[1] == "" {
			unifiedPath = parts[2]
			continue
		}
		for _, controller := range strings.Split(parts[1], ",") {
			if controller == "memory" {
				legacyPath = parts[2]
				break
			}
		}
	}

	locations := make([]cgroupMemoryLocation, 0, 2)
	for _, line := range strings.Split(string(mountInfoData), "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || separator+3 >= len(fields) {
			continue
		}
		mountRoot := unescapeMountInfoPath(fields[3])
		mountPath := unescapeMountInfoPath(fields[4])
		switch fields[separator+1] {
		case "cgroup2":
			if unifiedPath != "" {
				locations = append(locations, cgroupMemoryLocation{
					groupPath: unifiedPath,
					mountRoot: mountRoot,
					mountPath: mountPath,
					files:     []string{"memory.max", "memory.high"},
				})
			}
		case "cgroup":
			if legacyPath != "" && hasMountOption(fields[separator+3], "memory") {
				locations = append(locations, cgroupMemoryLocation{
					groupPath: legacyPath,
					mountRoot: mountRoot,
					mountPath: mountPath,
					files:     []string{"memory.limit_in_bytes", "memory.soft_limit_in_bytes"},
				})
			}
		}
	}
	return locations
}

func resolveCgroupPath(location cgroupMemoryLocation) (string, bool) {
	groupPath := filepath.Clean(filepath.FromSlash("/" + strings.TrimPrefix(location.groupPath, "/")))
	membershipRoot := filepath.Clean(filepath.FromSlash("/" + strings.TrimPrefix(location.mountRoot, "/")))
	var relative string
	switch {
	case groupPath == membershipRoot:
	case membershipRoot == string(filepath.Separator):
		relative = strings.TrimPrefix(groupPath, string(filepath.Separator))
	case strings.HasPrefix(groupPath, membershipRoot+string(filepath.Separator)):
		relative = strings.TrimPrefix(groupPath, membershipRoot+string(filepath.Separator))
	default:
		return "", false
	}
	resolved := filepath.Clean(filepath.Join(location.mountPath, relative))
	if !pathWithin(resolved, filepath.Clean(location.mountPath)) {
		return "", false
	}
	return resolved, true
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func hasMountOption(options, want string) bool {
	for _, option := range strings.Split(options, ",") {
		if option == want {
			return true
		}
	}
	return false
}

func unescapeMountInfoPath(value string) string {
	replacer := strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	)
	return replacer.Replace(value)
}

func readMemAvailable() (int64, bool) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
