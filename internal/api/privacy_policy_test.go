/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package api

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadPrivacyPolicyFileEnforcesPlainTextBoundary(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "privacy-policy.txt")
	require.NoError(t, os.WriteFile(path, []byte("Privacy policy\n"), 0o600))
	data, err := readPrivacyPolicyFile(path)
	require.NoError(t, err)
	assert.Equal(t, "Privacy policy\n", string(data))

	require.NoError(t, os.WriteFile(path, []byte{'p', 0, 'x'}, 0o600))
	_, err = readPrivacyPolicyFile(path)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(path, []byte{0xff, 0xfe}, 0o600))
	_, err = readPrivacyPolicyFile(path)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxPrivacyPolicyBytes+1), 0o600))
	_, err = readPrivacyPolicyFile(path)
	require.Error(t, err)
	_, err = readPrivacyPolicyFile(filepath.Join(directory, "missing.txt"))
	assert.True(t, errors.Is(err, os.ErrNotExist))
}
