/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package repositorygate serializes format/configuration changes with repository mutations.
package repositorygate

import (
	"slices"
	"sync"
)

const gateStripeCount = 64

var gateStripes [gateStripeCount]sync.RWMutex

func repositoryStripeIndex(repository string) uint32 {
	var hash uint32 = 2166136261
	for index := 0; index < len(repository); index++ {
		value := repository[index]
		if value >= 'A' && value <= 'Z' {
			value += 'a' - 'A'
		}
		hash ^= uint32(value)
		hash *= 16777619
	}
	return hash % gateStripeCount
}

// AcquireMutation allows concurrent operations for one repository while excluding configuration migration.
func AcquireMutation(repository string) func() {
	gate := &gateStripes[repositoryStripeIndex(repository)]
	gate.RLock()
	return gate.RUnlock
}

// AcquireMigration excludes repository mutations while an engine or storage configuration changes.
func AcquireMigration(repository string) func() {
	gate := &gateStripes[repositoryStripeIndex(repository)]
	gate.Lock()
	return gate.Unlock
}

// AcquireMutations holds distinct repository gates in order for a cross-repository operation.
func AcquireMutations(repositories ...string) func() {
	stripes := make([]uint32, 0, len(repositories))
	for _, repository := range repositories {
		stripes = append(stripes, repositoryStripeIndex(repository))
	}
	slices.Sort(stripes)
	stripes = slices.Compact(stripes)
	for _, stripe := range stripes {
		gateStripes[stripe].RLock()
	}
	return func() {
		for i := len(stripes) - 1; i >= 0; i-- {
			gateStripes[stripes[i]].RUnlock()
		}
	}
}

// AcquireAllMigrations excludes every repository mutation during one cross-repository account retirement.
func AcquireAllMigrations() func() {
	for index := range gateStripes {
		gateStripes[index].Lock()
	}
	return func() {
		for index := len(gateStripes) - 1; index >= 0; index-- {
			gateStripes[index].Unlock()
		}
	}
}
