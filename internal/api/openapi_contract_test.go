/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package api

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOpenAPIContractParsesAndIncludesPublicationReviewRoutes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "web", "assets", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	for _, path := range []string{
		"/api/reviews/{id}/files",
		"/api/reviews/{id}/files/{file_id}",
		"/api/settings/repositories/publication-reviews",
		"/api/settings/repositories/{name}/publication-review",
	} {
		if _, exists := document.Paths[path]; !exists {
			t.Fatalf("OpenAPI contract is missing %s", path)
		}
	}
}
