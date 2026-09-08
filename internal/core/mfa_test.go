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
	"encoding/base32"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTOTPStandardVectorsAndValidationWindow(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, vector := range []struct {
		timestamp int64
		code      string
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"},
		{1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	} {
		require.Equal(t, vector.code, TOTPCode(secret, vector.timestamp/30))
		require.Equal(t, vector.timestamp/30, VerifyTOTP(secret, vector.code, time.Unix(vector.timestamp, 0)))
	}
	now := time.Unix(2000000000, 0)
	for offset := int64(-1); offset <= 1; offset++ {
		require.Equal(t, now.Unix()/30+offset, VerifyTOTP(secret, TOTPCode(secret, now.Unix()/30+offset), now))
	}
	require.Equal(t, int64(-1), VerifyTOTP(secret, TOTPCode(secret, now.Unix()/30-2), now))
	for _, invalid := range []string{"", "12345", "1234567", "１２３４５６", " 12345"} {
		require.Equal(t, int64(-1), VerifyTOTP(secret, invalid, now))
	}
	require.Equal(t, "", TOTPCode("invalid", 1))
}
