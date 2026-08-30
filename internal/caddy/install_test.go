/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package caddy

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	validated []byte
	reloads   int
	reloadErr error
}

func (runner *recordingRunner) Validate(_, _ string, content []byte) error {
	runner.validated = append([]byte(nil), content...)
	return nil
}

func (runner *recordingRunner) Reload(_, _ string) error {
	runner.reloads++
	return runner.reloadErr
}

func TestInstallAppliesValidatedTransaction(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	caddyfile := filepath.Join(directory, "Caddyfile")
	configPath := filepath.Join(directory, "config.yaml")
	require.NoError(t, os.WriteFile(caddyfile, []byte("localhost { respond ok }\n"), 0644))
	require.NoError(t, os.WriteFile(configPath, []byte("server:\n  port: 3456\n"), 0600))
	binary, err := os.Executable()
	require.NoError(t, err)
	runner := &recordingRunner{}

	result, err := Install(Options{
		Hostname:      "packages.example.com",
		CaddyfilePath: caddyfile,
		ConfigPath:    configPath,
		CaddyBinary:   binary,
		CommandRunner: runner,
	})
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:3456", result.Upstream)
	require.True(t, result.Reloaded)
	require.True(t, result.RestartRequired)
	require.Equal(t, 1, runner.reloads)
	require.Contains(t, string(runner.validated), "packages.example.com")
	caddyBytes, err := os.ReadFile(caddyfile)
	require.NoError(t, err)
	require.Equal(t, runner.validated, caddyBytes)
	configBytes, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(configBytes), "host: 127.0.0.1")
	if runtime.GOOS != "windows" {
		configInfo, statErr := os.Stat(configPath)
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0600), configInfo.Mode().Perm())
	}
}

func TestReadOptionalFileEnforcesManagedConfigBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxManagedConfigBytes), 0o600))
	data, mode, exists, err := readOptionalFile(path)
	require.NoError(t, err)
	require.True(t, exists)
	require.Len(t, data, maxManagedConfigBytes)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), mode)
	}

	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxManagedConfigBytes+1), 0o600))
	_, _, _, err = readOptionalFile(path)
	require.ErrorContains(t, err, "4 MiB")
}

func TestInstallRollsBackBothFilesWhenReloadFails(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	caddyfile := filepath.Join(directory, "Caddyfile")
	configPath := filepath.Join(directory, "config.yaml")
	caddyOriginal := []byte("localhost { respond ok }\n")
	configOriginal := []byte("server:\n  port: 3456\n")
	require.NoError(t, os.WriteFile(caddyfile, caddyOriginal, 0644))
	require.NoError(t, os.WriteFile(configPath, configOriginal, 0600))
	binary, err := os.Executable()
	require.NoError(t, err)
	runner := &recordingRunner{reloadErr: errors.New("reload unavailable")}

	_, err = Install(Options{
		Hostname:      "packages.example.com",
		CaddyfilePath: caddyfile,
		ConfigPath:    configPath,
		CaddyBinary:   binary,
		CommandRunner: runner,
	})
	require.ErrorContains(t, err, "reload unavailable")
	caddyAfter, readErr := os.ReadFile(caddyfile)
	require.NoError(t, readErr)
	require.Equal(t, caddyOriginal, caddyAfter)
	configAfter, readErr := os.ReadFile(configPath)
	require.NoError(t, readErr)
	require.Equal(t, configOriginal, configAfter)
	require.Equal(t, 2, runner.reloads, "the restored Caddyfile should be reloaded once")
}

func TestDiscoverCaddyfilesResolvesExplicitFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "Caddyfile")
	require.NoError(t, os.WriteFile(path, []byte("localhost"), 0644))
	paths, err := DiscoverCaddyfiles(path)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	require.Equal(t, path, paths[0])
}
