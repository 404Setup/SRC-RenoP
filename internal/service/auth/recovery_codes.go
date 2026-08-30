/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"renop/internal/core"
)

const (
	recoveryCodeBytes       = 20
	recoverySelectorBytes   = 6
	recoveryArgonMemoryKiB  = 32 * 1024
	recoveryArgonIterations = 3
	recoveryArgonParallel   = 1
	recoveryArgonKeyLength  = 32
	recoveryArgonSaltLength = 16
)

var recoveryBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

type parsedRecoveryCode struct {
	selectorHash string
	value        []byte
	valid        bool
}

func recoverySelectorHash(value []byte) string {
	digest := sha256.Sum256(value[:recoverySelectorBytes])
	return hex.EncodeToString(digest[:])
}

func formatRecoveryCode(encoded string) string {
	var builder strings.Builder
	builder.Grow(len(encoded) + len(encoded)/4 + 4)
	builder.WriteString("RNP-")
	for index := 0; index < len(encoded); index += 4 {
		if index > 0 {
			builder.WriteByte('-')
		}
		builder.WriteString(encoded[index : index+4])
	}
	return builder.String()
}

func parseRecoveryCode(value string) parsedRecoveryCode {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.TrimPrefix(normalized, "RNP")
	if len(normalized) != 32 {
		digest := sha256.Sum256([]byte(normalized))
		return parsedRecoveryCode{
			selectorHash: recoverySelectorHash(digest[:recoveryCodeBytes]),
			value:        append([]byte(nil), digest[:recoveryCodeBytes]...),
		}
	}
	decoded, err := recoveryBase32.DecodeString(normalized)
	if err != nil || len(decoded) != recoveryCodeBytes {
		digest := sha256.Sum256([]byte(normalized))
		return parsedRecoveryCode{
			selectorHash: recoverySelectorHash(digest[:recoveryCodeBytes]),
			value:        append([]byte(nil), digest[:recoveryCodeBytes]...),
		}
	}
	return parsedRecoveryCode{
		selectorHash: recoverySelectorHash(decoded),
		value:        decoded,
		valid:        true,
	}
}

func encodeRecoveryCodeHash(value []byte) (string, error) {
	salt := make([]byte, recoveryArgonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey(value, salt, recoveryArgonIterations, recoveryArgonMemoryKiB,
		recoveryArgonParallel, recoveryArgonKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, recoveryArgonMemoryKiB, recoveryArgonIterations, recoveryArgonParallel,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func decodeRecoveryCodeHash(encoded string) (memory uint32, iterations uint32, parallel uint8,
	salt, expected []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return 0, 0, 0, nil, nil, errors.New("invalid recovery-code hash")
	}
	var parallelValue uint64
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelValue); err != nil {
		return 0, 0, 0, nil, nil, errors.New("invalid recovery-code parameters")
	}
	if memory < 19*1024 || memory > 128*1024 || iterations < 2 || iterations > 10 ||
		parallelValue < 1 || parallelValue > 4 {
		return 0, 0, 0, nil, nil, errors.New("unsafe recovery-code parameters")
	}
	parallel = uint8(parallelValue)
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return 0, 0, 0, nil, nil, errors.New("invalid recovery-code salt")
	}
	expected, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != recoveryArgonKeyLength {
		return 0, 0, 0, nil, nil, errors.New("invalid recovery-code verifier")
	}
	return memory, iterations, parallel, salt, expected, nil
}

func verifyRecoveryCode(value []byte, encoded string) bool {
	memory, iterations, parallel, salt, expected, err := decodeRecoveryCodeHash(encoded)
	if err != nil {
		consumeDummyRecoveryWork(value)
		return false
	}
	actual := argon2.IDKey(value, salt, iterations, memory, parallel, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func consumeDummyRecoveryWork(value []byte) {
	var salt [recoveryArgonSaltLength]byte
	actual := argon2.IDKey(value, salt[:], recoveryArgonIterations, recoveryArgonMemoryKiB,
		recoveryArgonParallel, recoveryArgonKeyLength)
	var expected [recoveryArgonKeyLength]byte
	_ = subtle.ConstantTimeCompare(actual, expected[:])
}

func generateRecoveryCodeSet() ([]string, []core.RecoveryCodeHash, error) {
	displayCodes := make([]string, 0, core.RecoveryCodeCount)
	hashes := make([]core.RecoveryCodeHash, 0, core.RecoveryCodeCount)
	seen := make(map[string]struct{}, core.RecoveryCodeCount)
	createdAt := time.Now().UnixMilli()
	for len(displayCodes) < core.RecoveryCodeCount {
		value := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(value); err != nil {
			return nil, nil, err
		}
		selectorHash := recoverySelectorHash(value)
		if _, duplicate := seen[selectorHash]; duplicate {
			continue
		}
		passwordHash, err := encodeRecoveryCodeHash(value)
		if err != nil {
			return nil, nil, err
		}
		seen[selectorHash] = struct{}{}
		displayCodes = append(displayCodes, formatRecoveryCode(recoveryBase32.EncodeToString(value)))
		hashes = append(hashes, core.RecoveryCodeHash{
			SelectorHash: selectorHash,
			PasswordHash: passwordHash,
			CreatedAt:    createdAt,
		})
	}
	return displayCodes, hashes, nil
}
