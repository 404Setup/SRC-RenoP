/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"os"
	"path/filepath"
	"testing"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/gpg"
	"renop/internal/service/index"
)

func TestCreateFileDetailsDoesNotExposeBlockedPhysicalFile(t *testing.T) {
	artifactPath := filepath.Join(t.TempDir(), "releases", "org", "example", "demo.jar")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("quarantined"), 0644); err != nil {
		t.Fatal(err)
	}

	state := core.NewAppState()
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	state.Inner.FileIndex.InsertFile(artifactPath, index.FileInfo{Size: 11, ModTime: 1})
	state.Inner.FileIndex.BlockFile(artifactPath)

	if details := CreateFileDetails(state, artifactPath, false); details != nil {
		t.Fatalf("blocked physical file was exposed: %+v", details)
	}
}

func TestAnnotateGPGSignaturesMarksVerifiedArtifacts(t *testing.T) {
	db, err := database.InitDB(config.DatabaseConfig{
		Driver:       "sqlite",
		Dsn:          filepath.Join(t.TempDir(), "details.db"),
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	state := core.NewAppState()
	state.Inner.DB = db
	details := &FileDetails{
		Type: FileDetailsTypeDirectory,
		Files: []FileDetails{
			{Type: FileDetailsTypeFile, Name: "demo.jar"},
			{Type: FileDetailsTypeFile, Name: "demo.pom"},
			{Type: FileDetailsTypeFile, Name: "README.txt"},
		},
	}
	if err := db.SaveGPGSignature(&core.GPGSignature{
		ArtifactKey:  gpg.ArtifactKey("releases", "com/example/demo/demo.jar"),
		Repository:   "releases",
		ArtifactPath: "com/example/demo/demo.jar",
	}); err != nil {
		t.Fatal(err)
	}

	if err := annotateGPGSignatures(state, "releases", "com/example/demo", details); err != nil {
		t.Fatal(err)
	}
	if !details.Files[0].Signed {
		t.Fatal("verified artifact was not marked signed")
	}
	if details.Files[1].Signed || details.Files[2].Signed {
		t.Fatal("unverified files were marked signed")
	}
}
