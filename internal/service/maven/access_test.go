/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"testing"

	"renop/internal/config"
)

func TestHiddenRepositoryDiscoveryRequiresPermission(t *testing.T) {
	repository := &config.Repository{
		Name: "maven-hidden", Format: config.RepositoryFormatMaven, Visibility: "HIDDEN",
	}

	allowed, err := CanReadRepository(nil, &config.User{Username: "guest"}, repository, "org/example/demo.pom", false)
	if err != nil || !allowed {
		t.Fatalf("hidden Maven direct file read access = %v, err = %v", allowed, err)
	}

	allowed, err = CanReadRepository(nil, &config.User{Username: "guest"}, repository, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("guest unexpectedly discovered hidden Maven repository root")
	}

	allowed, err = CanReadRepository(nil, &config.User{Username: "manager", Roles: []string{"manager"}}, repository, "", true)
	if err != nil || !allowed {
		t.Fatalf("manager hidden Maven root access = %v, err = %v", allowed, err)
	}
}
