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
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxSoftMemoryLimitDoesNotPanic(t *testing.T) {
	if limit, ok := linuxSoftMemoryLimit(); ok {
		if limit < 64<<20 {
			t.Fatalf("limit too small: %d", limit)
		}
		t.Logf("soft memory limit = %d MiB", limit>>20)
	} else {
		t.Log("no soft memory limit available on this host")
	}
}

func TestInitLinuxMemoryTuningIdempotent(t *testing.T) {
	InitLinuxMemoryTuning()
	InitLinuxMemoryTuning()
}

func TestLinuxRuntimeDebugDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		want        string
		wantChanged bool
	}{
		{name: "empty", want: "disablethp=1", wantChanged: true},
		{name: "preserve others", input: "madvdontneed=1", want: "madvdontneed=1,disablethp=1", wantChanged: true},
		{name: "explicit enable", input: "disablethp=1", want: "disablethp=1"},
		{name: "explicit disable", input: "gctrace=1,disablethp=0", want: "gctrace=1,disablethp=0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := linuxRuntimeDebug(tt.input)
			if got != tt.want || changed != tt.wantChanged {
				t.Fatalf("linuxRuntimeDebug(%q) = %q, %v; want %q, %v", tt.input, got, changed, tt.want, tt.wantChanged)
			}
		})
	}
}

func TestReadCgroupV2NestedLimits(t *testing.T) {
	mountPath := filepath.Join(t.TempDir(), "cgroup2")
	groupPath := filepath.Join(mountPath, "system.slice", "renop.service")
	if err := os.MkdirAll(groupPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestLimit(t, filepath.Join(mountPath, "memory.max"), "536870912\n")
	writeTestLimit(t, filepath.Join(groupPath, "memory.max"), "max\n")
	writeTestLimit(t, filepath.Join(groupPath, "memory.high"), "268435456\n")

	cgroupData := []byte("0::/system.slice/renop.service\n")
	mountInfo := []byte(fmt.Sprintf("29 23 0:26 / %s rw,nosuid,nodev,noexec,relatime - cgroup2 cgroup rw\n", mountPath))
	limit, ok := readCgroupMemoryLimitFrom(cgroupData, mountInfo)
	if !ok || limit != 256<<20 {
		t.Fatalf("limit: got %d, %v want %d, true", limit, ok, 256<<20)
	}
}

func TestReadCgroupV1MemoryController(t *testing.T) {
	mountPath := filepath.Join(t.TempDir(), "memory")
	groupPath := filepath.Join(mountPath, "docker", "renop")
	if err := os.MkdirAll(groupPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestLimit(t, filepath.Join(groupPath, "memory.limit_in_bytes"), "134217728\n")
	writeTestLimit(t, filepath.Join(groupPath, "memory.soft_limit_in_bytes"), "9223372036854771712\n")

	cgroupData := []byte("5:cpu,cpuacct:/docker/renop\n6:memory:/docker/renop\n")
	mountInfo := []byte(fmt.Sprintf("30 23 0:27 / %s rw,relatime - cgroup cgroup rw,memory\n", mountPath))
	limit, ok := readCgroupMemoryLimitFrom(cgroupData, mountInfo)
	if !ok || limit != 128<<20 {
		t.Fatalf("limit: got %d, %v want %d, true", limit, ok, 128<<20)
	}
}

func TestResolveNamespacedCgroupMount(t *testing.T) {
	location := cgroupMemoryLocation{
		groupPath: "/user.slice/user-1000.slice/session.scope",
		mountRoot: "/user.slice/user-1000.slice",
		mountPath: "/sys/fs/cgroup",
	}
	got, ok := resolveCgroupPath(location)
	want := filepath.Join("/sys/fs/cgroup", "session.scope")
	if !ok || got != want {
		t.Fatalf("resolved path: got %q, %v want %q, true", got, ok, want)
	}
}

func writeTestLimit(t *testing.T, name, value string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}
