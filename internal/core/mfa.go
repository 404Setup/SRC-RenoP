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
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// ErrMFAInvalid indicates a stale challenge, rejected code, or changed authentication policy.
var ErrMFAInvalid = errors.New("multi-factor verification is invalid")

// MFAState is private persisted second-factor state. Secret contains authenticated ciphertext.
type MFAState struct {
	PasswordLoginEnabled                             bool
	UserID, Secret, Revision, Snapshot, PasswordHash string
	GitHubID                                         int64
	Passkey                                          bool
	LastStep, WindowStart                            int64
	Failures                                         int
}

// Enabled reports whether browser login requires an additional factor.
func (m MFAState) Enabled() bool { return m.Passkey || m.Secret != "" }

// TOTPCode computes the six-digit HMAC-SHA1 code for a 30-second counter.
func TOTPCode(secret string, step int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil || len(key) < 20 || step < 0 {
		return ""
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
}

// VerifyTOTP returns a matching counter within one time step, or -1 for an invalid code.
func VerifyTOTP(secret, code string, now time.Time) int64 {
	if len(code) != 6 {
		return -1
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return -1
		}
	}
	matched := int64(-1)
	for step := now.Unix()/30 - 1; step <= now.Unix()/30+1; step++ {
		if expected := TOTPCode(secret, step); expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			matched = step
		}
	}
	return matched
}
