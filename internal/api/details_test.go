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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/gpg"
	"renop/internal/service/index"
	"renop/internal/testutil"
	"renop/pkg/pb"
)

func TestHiddenRepositoryIsNotDiscoverableButDirectFileRemainsReadable(t *testing.T) {
	storagePath := testutil.TempDir(t)
	hiddenFile := filepath.Join(storagePath, "hidden", "known", "artifact.txt")
	publicFile := filepath.Join(storagePath, "public", "artifact.txt")
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Maven.Repositories = map[string]*config.Repository{
		"hidden": {Name: "hidden", Format: config.RepositoryFormatFiles, Visibility: "HIDDEN"},
		"public": {Name: "public", Format: config.RepositoryFormatFiles, Visibility: "PUBLIC"},
	}
	state.Inner.Config.Store(cfg)
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	for _, file := range []string{hiddenFile, publicFile} {
		state.Inner.FileIndex.EnsureParentDirs(file)
		state.Inner.FileIndex.InsertFile(file, index.FileInfo{Size: 8, ModTime: 1})
	}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		if c.Get("X-Test-Role") == "manager" {
			c.Locals("user", &config.User{Username: "manager", Roles: []string{"manager"}})
		}
		return c.Next()
	})
	app.Get("/api/repositories/details", func(c fiber.Ctx) error { return GetDetailsAllRepos(c, state) })
	app.Get("/api/repositories/details/:repo_name/*", func(c fiber.Ctx) error { return GetDetails(c, state) })

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/repositories/details", nil))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var listing pb.FileDetails
	if err := proto.Unmarshal(body, &listing); err != nil {
		t.Fatal(err)
	}
	if len(listing.Files) != 1 || listing.Files[0].Name != "public" {
		t.Fatalf("hidden repository leaked through discovery: %+v", listing.Files)
	}

	managerRequest := httptest.NewRequest(http.MethodGet, "/api/repositories/details", nil)
	managerRequest.Header.Set("X-Test-Role", "manager")
	managerResponse, err := app.Test(managerRequest)
	if err != nil {
		t.Fatal(err)
	}
	managerBody, err := io.ReadAll(managerResponse.Body)
	_ = managerResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var managerListing pb.FileDetails
	if err := proto.Unmarshal(managerBody, &managerListing); err != nil {
		t.Fatal(err)
	}
	managerRepositories := make(map[string]bool, len(managerListing.Files))
	for _, repository := range managerListing.Files {
		managerRepositories[repository.Name] = true
	}
	if len(managerRepositories) != 2 || !managerRepositories["hidden"] || !managerRepositories["public"] {
		t.Fatalf("manager repository discovery = %+v, want hidden and public", managerListing.Files)
	}

	direct, err := app.Test(httptest.NewRequest(http.MethodGet,
		"/api/repositories/details/hidden/known/artifact.txt", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer direct.Body.Close()
	if direct.StatusCode != fiber.StatusOK {
		t.Fatalf("known hidden repository path status = %d, want 200", direct.StatusCode)
	}
	directBody, err := io.ReadAll(direct.Body)
	if err != nil {
		t.Fatal(err)
	}
	var detail pb.FileDetails
	if err := proto.Unmarshal(directBody, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Name != "artifact.txt" || detail.Type != string(FileDetailsTypeFile) {
		t.Fatalf("unexpected hidden file details: name=%q type=%q", detail.Name, detail.Type)
	}
}

func TestCreateFileDetailsDoesNotExposeBlockedPhysicalFile(t *testing.T) {
	artifactPath := filepath.Join(testutil.TempDir(t), "releases", "org", "example", "demo.jar")
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
		Dsn:          filepath.Join(testutil.TempDir(t), "details.db"),
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
