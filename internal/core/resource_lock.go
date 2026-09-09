/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"errors"
	"runtime"
	"strings"
)

const (
	ResourceLockWrite  = "write"
	ResourceLockRead   = "read"
	ResourceLockManual = "manual"
	ResourceLockSystem = "system"
)

var (
	ErrResourceLocked         = errors.New("resource is locked")
	ErrResourceLockInvalid    = errors.New("invalid resource lock")
	ErrResourceLockPermission = errors.New("resource lock permission denied")
)

// ResourceLockTarget identifies a package or one of its versions.
type ResourceLockTarget struct {
	Format     string `json:"format"`
	Repository string `json:"repository"`
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
}

// ResourceLock exposes the public reason for an independently removable restriction.
type ResourceLock struct {
	ResourceLockTarget
	Source   string `json:"source"`
	Mode     string `json:"mode"`
	Reason   string `json:"reason"`
	LockedAt int64  `json:"locked_at"`
}

// ValidResourceLockReason accepts the public, localized moderation reasons.
func ValidResourceLockReason(reason string) bool {
	switch reason {
	case "hold", "prohibited", "expired", "trojan", "abuse", "dmca", "reup", "squatting", "quality":
		return true
	default:
		return false
	}
}

// ReadLocked reports whether these effective locks restrict metadata and file reads.
func ReadLocked(locks []*ResourceLock) bool {
	for _, lock := range locks {
		if lock.Mode == ResourceLockRead {
			return true
		}
	}
	return false
}

// ResourceLockVersionKey includes case aliases that address the same Cargo file on Windows.
func ResourceLockVersionKey(format, version string) string {
	if runtime.GOOS == "windows" && format == "cargo" {
		return strings.ToLower(version)
	}
	return version
}
