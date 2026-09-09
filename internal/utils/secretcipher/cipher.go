/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package secretcipher provides authenticated encryption for server-owned secrets.
package secretcipher

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"

	"github.com/emmansun/base64"
)

// New creates an authenticated cipher with automatic random nonces from a 256-bit key.
func New(key string) (cipher.AEAD, error) {
	raw, err := base64.RawStdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("invalid encryption key")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
