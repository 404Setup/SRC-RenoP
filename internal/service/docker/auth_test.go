/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeImageName(t *testing.T) {
	for input, expected := range map[string]string{
		"team/service": "team/service",
		"Library/App":  "library/app",
		"a/b_c.d-e":    "a/b_c.d-e",
	} {
		normalized, valid := NormalizeImageName(input)
		assert.True(t, valid, input)
		assert.Equal(t, expected, normalized)
	}
	for _, input := range []string{"", "/", "team//service", "team/service:", "team/service@sha256", "../service"} {
		_, valid := NormalizeImageName(input)
		assert.False(t, valid, input)
	}
}
