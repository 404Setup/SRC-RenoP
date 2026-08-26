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

import "sync"

const gateStripeCount = 64

var gateStripes [gateStripeCount]sync.RWMutex

func repositoryStripe(repository string) *sync.RWMutex {
	var hash uint32 = 2166136261
	for index := 0; index < len(repository); index++ {
		value := repository[index]
		if value >= 'A' && value <= 'Z' {
			value += 'a' - 'A'
		}
		hash ^= uint32(value)
		hash *= 16777619
	}
	return &gateStripes[hash%gateStripeCount]
}

// AcquireMutation allows concurrent operations for one repository while excluding configuration migration.
func AcquireMutation(repository string) func() {
	gate := repositoryStripe(repository)
	gate.RLock()
	return gate.RUnlock
}

// AcquireMigration excludes repository mutations while an engine or storage configuration changes.
func AcquireMigration(repository string) func() {
	gate := repositoryStripe(repository)
	gate.Lock()
	return gate.Unlock
}
