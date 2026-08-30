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
	"testing"

	"renop/internal/config"
)

func TestArtifactURL(t *testing.T) {
	cargoRepo := &config.Repository{Format: config.RepositoryFormatCargo}
	mirror := config.Mirror{
		URL:         "https://index.crates.io/",
		ArtifactURL: "https://static.crates.io/crates/{crate}/{crate}-{version}.crate",
	}
	actual := ArtifactURL(cargoRepo, mirror, "api/v1/crates/serde/1.0.203/download")
	expected := "https://static.crates.io/crates/serde/serde-1.0.203.crate"
	if actual != expected {
		t.Fatalf("ArtifactURL() = %q, want %q", actual, expected)
	}

	mavenRepo := &config.Repository{Format: config.RepositoryFormatMaven}
	actual = ArtifactURL(mavenRepo, mirror, "org/example/demo/1.0/demo-1.0+all.jar")
	expected = "https://index.crates.io/org/example/demo/1.0/demo-1.0+all.jar"
	if actual != expected {
		t.Fatalf("Maven ArtifactURL() = %q, want %q", actual, expected)
	}
}
