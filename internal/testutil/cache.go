/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package testutil

import (
	"os"
	"testing"

	"renop/internal/cache"
)

// RemoteCache enables existing cache contracts against an explicitly configured test instance.
func RemoteCache(t *testing.T) *cache.Remote {
	t.Helper()
	address := os.Getenv("RENOP_TEST_CACHE_ADDRESS")
	if address == "" {
		return nil
	}
	mode := os.Getenv("RENOP_TEST_CACHE_MODE")
	if mode == "" {
		mode = "redis"
	}
	remote, err := cache.Open(cache.Config{Mode: mode, Address: address, Password: os.Getenv("RENOP_TEST_CACHE_PASSWORD")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := remote.Close(); err != nil {
			t.Error(err)
		}
	})
	return remote
}
