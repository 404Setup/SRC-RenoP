/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"path/filepath"
	"testing"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestNPMVisibilityRequiresRepositoryAndPackageAccess(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "npm-access.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, username := range []string{"alice", "bob"} {
		if err := db.SaveToken(&core.AccessToken{Name: username, Permissions: []string{"base"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CreateNPMPackage("npm", "@team/private", "alice", true, 1); err != nil {
		t.Fatal(err)
	}
	state := core.NewAppState()
	state.Inner.DB = db
	repository := &config.Repository{Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC"}
	allowed, err := CanReadPackage(state, &config.User{Username: "alice"}, repository, "@team/private")
	if err != nil || !allowed {
		t.Fatalf("npm package owner access = %v, err = %v", allowed, err)
	}
	allowed, err = CanReadPackage(state, &config.User{Username: "bob"}, repository, "@team/private")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("unrelated user unexpectedly received private npm package access")
	}

	hidden := &config.Repository{Name: "npm", Format: config.RepositoryFormatNPM, Visibility: "HIDDEN"}
	allowed, err = CanReadRepository(state, &config.User{Username: "guest"}, hidden, "public-package", false)
	if err != nil || !allowed {
		t.Fatalf("hidden npm direct package access = %v, err = %v", allowed, err)
	}
	allowed, err = CanReadRepository(state, &config.User{Username: "guest"}, hidden, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("guest unexpectedly discovered hidden npm repository root")
	}
	allowed, err = CanReadRepository(state,
		&config.User{Username: "manager", Roles: []string{"manager"}}, hidden, "", true)
	if err != nil || !allowed {
		t.Fatalf("manager hidden npm root access = %v, err = %v", allowed, err)
	}
}
