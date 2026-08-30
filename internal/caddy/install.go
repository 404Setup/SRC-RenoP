/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package caddy configures RenoP behind a locally managed Caddy reverse proxy.
package caddy

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"renop/internal/utils"
)

// Options controls a Caddy installation transaction.
type Options struct {
	Hostname      string
	CaddyfilePath string
	ConfigPath    string
	CaddyBinary   string
	SkipReload    bool
	CommandRunner CommandRunner
}

// Result describes files updated by a successful Caddy installation.
type Result struct {
	Hostname        string
	CaddyfilePath   string
	ConfigPath      string
	Upstream        string
	Reloaded        bool
	RestartRequired bool
}

// CommandRunner validates and reloads a Caddy configuration.
type CommandRunner interface {
	Validate(binaryPath, caddyfilePath string, content []byte) error
	Reload(binaryPath, caddyfilePath string) error
}

type execCommandRunner struct{}

func (execCommandRunner) Validate(binaryPath, caddyfilePath string, content []byte) (err error) {
	directory := filepath.Dir(caddyfilePath)
	temporary, err := os.CreateTemp(directory, ".renop-caddy-validate-*")
	if err != nil {
		return fmt.Errorf("create temporary Caddy configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary Caddy configuration: %w", removeErr))
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return errors.Join(fmt.Errorf("secure temporary Caddy configuration: %w", err), temporary.Close())
	}
	if _, err := temporary.Write(content); err != nil {
		return errors.Join(fmt.Errorf("write temporary Caddy configuration: %w", err), temporary.Close())
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary Caddy configuration: %w", err)
	}
	command := exec.Command(binaryPath, "validate", "--config", temporaryPath, "--adapter", "caddyfile")
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return commandError("validate Caddy configuration", output, err)
	}
	return nil
}

func (execCommandRunner) Reload(binaryPath, caddyfilePath string) error {
	command := exec.Command(binaryPath, "reload", "--config", caddyfilePath, "--adapter", "caddyfile")
	command.Dir = filepath.Dir(caddyfilePath)
	output, err := command.CombinedOutput()
	if err != nil {
		return commandError("reload Caddy", output, err)
	}
	return nil
}

// Install validates and atomically applies a RenoP reverse-proxy configuration.
func Install(options Options) (Result, error) {
	var result Result
	hostname, err := NormalizeHostname(options.Hostname)
	if err != nil {
		return result, err
	}
	caddyfiles, err := DiscoverCaddyfiles(options.CaddyfilePath)
	if err != nil {
		return result, err
	}
	if len(caddyfiles) != 1 {
		return result, errors.New("exactly one Caddyfile must be selected")
	}
	caddyfilePath := caddyfiles[0]
	configPath, err := resolveConfigPath(options.ConfigPath)
	if err != nil {
		return result, err
	}

	caddyOriginal, caddyMode, err := readExistingFile(caddyfilePath)
	if err != nil {
		return result, fmt.Errorf("read Caddyfile: %w", err)
	}
	configOriginal, configMode, configExists, err := readOptionalFile(configPath)
	if err != nil {
		return result, fmt.Errorf("read RenoP configuration: %w", err)
	}
	configUpdated, port, err := BuildRenoPConfig(configOriginal, hostname)
	if err != nil {
		return result, err
	}
	caddyUpdated, err := BuildCaddyfile(caddyOriginal, hostname, port)
	if err != nil {
		return result, err
	}

	binaryPath, err := DiscoverCaddyBinary(options.CaddyBinary)
	if err != nil {
		return result, err
	}
	if binaryPath == "" && !options.SkipReload {
		return result, errors.New("Caddy executable was not found; pass --caddy-binary or use --skip-reload for an offline configuration")
	}
	runner := options.CommandRunner
	if runner == nil {
		runner = execCommandRunner{}
	}
	if binaryPath != "" {
		if err := runner.Validate(binaryPath, caddyfilePath, caddyUpdated); err != nil {
			return result, err
		}
	}

	configChanged := !bytes.Equal(configOriginal, configUpdated)
	caddyChanged := !bytes.Equal(caddyOriginal, caddyUpdated)
	if configChanged {
		if err := replaceIfUnchanged(configPath, configOriginal, configUpdated, 0600, configExists); err != nil {
			return result, fmt.Errorf("write RenoP configuration: %w", err)
		}
	}
	if caddyChanged {
		if err := replaceIfUnchanged(caddyfilePath, caddyOriginal, caddyUpdated, caddyMode, true); err != nil {
			rollbackErr := rollbackFile(configPath, configUpdated, configOriginal, configMode, configExists, configChanged)
			return result, errors.Join(fmt.Errorf("write Caddyfile: %w", err), rollbackErr)
		}
	}

	reloaded := false
	if !options.SkipReload {
		if err := runner.Reload(binaryPath, caddyfilePath); err != nil {
			caddyRollback := rollbackFile(caddyfilePath, caddyUpdated, caddyOriginal, caddyMode, true, caddyChanged)
			configRollback := rollbackFile(configPath, configUpdated, configOriginal, configMode, configExists, configChanged)
			var restoredReload error
			if caddyRollback == nil && binaryPath != "" {
				if reloadErr := runner.Reload(binaryPath, caddyfilePath); reloadErr != nil {
					restoredReload = fmt.Errorf("reload restored Caddy configuration: %w", reloadErr)
				}
			}
			return result, errors.Join(err, caddyRollback, configRollback, restoredReload)
		}
		reloaded = true
	}

	result = Result{
		Hostname:        hostname,
		CaddyfilePath:   caddyfilePath,
		ConfigPath:      configPath,
		Upstream:        fmt.Sprintf("127.0.0.1:%d", port),
		Reloaded:        reloaded,
		RestartRequired: configChanged,
	}
	return result, nil
}

func resolveConfigPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = strings.TrimSpace(os.Getenv("RENOP_CONFIG"))
	}
	if path == "" {
		path = "config.yaml"
	}
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve RenoP configuration path: %w", err)
	}
	if directInfo, err := os.Lstat(absPath); err == nil {
		resolved, resolveErr := evalSymlinks(absPath, directInfo)
		if resolveErr != nil {
			return "", fmt.Errorf("resolve RenoP configuration path: %w", resolveErr)
		}
		absPath = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect RenoP configuration path: %w", err)
	} else {
		parentPath := filepath.Dir(absPath)
		parentInfo, statErr := os.Lstat(parentPath)
		if statErr != nil {
			return "", fmt.Errorf("inspect RenoP configuration directory: %w", statErr)
		}
		parent, resolveErr := evalSymlinks(parentPath, parentInfo)
		if resolveErr != nil {
			return "", fmt.Errorf("resolve RenoP configuration directory: %w", resolveErr)
		}
		absPath = filepath.Join(parent, filepath.Base(absPath))
	}
	if info, err := os.Stat(absPath); err == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("RenoP configuration %s is not a regular file", absPath)
	}
	return absPath, nil
}

func readExistingFile(path string) ([]byte, os.FileMode, error) {
	data, mode, exists, err := readOptionalFile(path)
	if err != nil {
		return nil, 0, err
	}
	if !exists {
		return nil, 0, os.ErrNotExist
	}
	return data, mode, nil
}

func readOptionalFile(path string) ([]byte, os.FileMode, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0600, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return data, info.Mode().Perm(), true, nil
}

func replaceIfUnchanged(path string, before, after []byte, mode os.FileMode, existed bool) error {
	current, _, currentExists, err := readOptionalFile(path)
	if err != nil {
		return err
	}
	if currentExists != existed || !bytes.Equal(current, before) {
		return errors.New("file changed while the Caddy installation was being prepared")
	}
	return atomicReplace(path, after, mode)
}

func rollbackFile(path string, expected, original []byte, mode os.FileMode, existed, changed bool) error {
	if !changed {
		return nil
	}
	current, _, currentExists, err := readOptionalFile(path)
	if err != nil {
		return fmt.Errorf("inspect %s before rollback: %w", path, err)
	}
	if !currentExists || !bytes.Equal(current, expected) {
		return fmt.Errorf("refuse to roll back %s because it changed after installation", path)
	}
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove newly created %s during rollback: %w", path, err)
		}
		return nil
	}
	if err := atomicReplace(path, original, mode); err != nil {
		return fmt.Errorf("restore %s: %w", path, err)
	}
	return nil
}

func atomicReplace(path string, data []byte, mode os.FileMode) (err error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".renop-write-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if _, err := temporary.Write(data); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Sync(); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := utils.SafeRename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func commandError(action string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if len(detail) > 4096 {
		detail = detail[:4096] + "..."
	}
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}
