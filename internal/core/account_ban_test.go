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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountBanValidationAndExpiry(t *testing.T) {
	reason, valid := NormalizeAccountBanReason("  repeated abuse  ")
	require.True(t, valid)
	assert.Equal(t, "repeated abuse", reason)
	_, valid = NormalizeAccountBanReason("line\nbreak")
	assert.False(t, valid)
	_, valid = NormalizeAccountBanReason(strings.Repeat("x", MaxAccountBanReasonRunes+1))
	assert.False(t, valid)

	permanent := &AccountBan{Reason: reason, CreatedAt: 100}
	assert.True(t, permanent.IsActive(1000))
	expiresAt := int64(200)
	temporary := &AccountBan{Reason: reason, CreatedAt: 100, ExpiresAt: &expiresAt}
	assert.True(t, temporary.IsActive(199))
	assert.False(t, temporary.IsActive(200))
	cloned := temporary.Clone()
	*cloned.ExpiresAt = 300
	assert.Equal(t, int64(200), *temporary.ExpiresAt)
}
