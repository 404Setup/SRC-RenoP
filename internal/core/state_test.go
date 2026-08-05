/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"strconv"
	"testing"

	"renop/internal/config"
)

func TestMetadataCacheIsStrictlyBounded(t *testing.T) {
	state := NewAppState()
	for i := range maxMetadataCacheEntries + 1 {
		state.StoreMetadataCache(strconv.Itoa(i), &config.Metadata{})
	}

	if got := state.Inner.MetadataCacheEntries.Load(); got != maxMetadataCacheEntries {
		t.Fatalf("metadata cache count = %d, want %d", got, maxMetadataCacheEntries)
	}
	if _, ok := state.Inner.MetadataCache.Load(strconv.Itoa(maxMetadataCacheEntries)); ok {
		t.Fatal("metadata cache accepted an entry beyond its limit")
	}

	state.DeleteMetadataCache("0")
	state.StoreMetadataCache("replacement", &config.Metadata{})
	if _, ok := state.Inner.MetadataCache.Load("replacement"); !ok {
		t.Fatal("metadata cache did not reuse capacity after invalidation")
	}
}
