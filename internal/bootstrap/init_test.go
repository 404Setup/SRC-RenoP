/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/index"
)

func TestStartServicesRejectsInvalidStoragePath(t *testing.T) {
	directory := t.TempDir()
	storagePath := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(storagePath, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndex()
	services, err := StartServices(state, BootstrapContext{IndexPath: filepath.Join(directory, "index.json")})
	if err == nil || services != nil {
		t.Fatalf("StartServices = (%v, %v), want nil runtime and storage error", services, err)
	}
}
