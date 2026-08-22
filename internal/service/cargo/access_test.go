/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"path/filepath"
	"testing"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestPrivateCargoMembershipGrantsRegistryReadAccess(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "cargo-access.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	state := core.NewAppState()
	state.Inner.DB = db
	packageRecord := &core.CargoPackage{
		Repository: "cargo", Name: "demo", NormalizedName: "demo", CreatedAt: 1, UpdatedAt: 1,
	}
	version := &core.CargoVersion{
		Repository: "cargo", Package: "demo", Version: "1.0.0", Publisher: "alice", CreatedAt: 1,
	}
	if err := db.RecordCargoPublication(packageRecord, version, "alice"); err != nil {
		t.Fatal(err)
	}

	repo := &config.Repository{Name: "cargo", Format: config.RepositoryFormatCargo, Visibility: "PRIVATE"}
	allowed, err := CanReadRepository(state, &config.User{Username: "alice"}, repo, "config.json", false)
	if err != nil || !allowed {
		t.Fatalf("Cargo package member read access = %v, err = %v", allowed, err)
	}
	allowed, err = CanReadRepository(state, &config.User{Username: "bob"}, repo, "config.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("unrelated user unexpectedly received private Cargo registry access")
	}

	repo.Format = config.RepositoryFormatMaven
	allowed, err = CanReadRepository(state, &config.User{Username: "alice"}, repo, "artifact.jar", false)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("Cargo membership must not grant access to a Maven repository")
	}
}
