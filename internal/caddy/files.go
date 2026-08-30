/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package caddy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// DiscoverCaddyfiles returns existing regular Caddyfiles in deterministic priority order.
func DiscoverCaddyfiles(explicitPath string) ([]string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		path, err := resolveRegularFile(explicitPath)
		if err != nil {
			return nil, fmt.Errorf("resolve Caddyfile: %w", err)
		}
		return []string{path}, nil
	}

	candidates := make([]string, 0, 16)
	if path := strings.TrimSpace(os.Getenv("CADDYFILE")); path != "" {
		candidates = append(candidates, path)
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			candidates = append(candidates, filepath.Join(dir, "Caddyfile"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "Caddyfile"))
	}
	if binary, err := exec.LookPath(caddyExecutableName()); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(binary), "Caddyfile"))
	}
	candidates = append(candidates, platformCaddyfileCandidates()...)

	seen := make(map[string]struct{}, len(candidates))
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		path, err := resolveRegularFile(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect Caddyfile candidate %q: %w", candidate, err)
		}
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, path)
	}
	if len(paths) > 1 {
		preferred := paths[0]
		sort.Strings(paths)
		for i, path := range paths {
			if path == preferred {
				paths[0], paths[i] = paths[i], paths[0]
				break
			}
		}
	}
	return paths, nil
}

// DiscoverCaddyBinary resolves an explicit or installed Caddy executable.
func DiscoverCaddyBinary(explicitPath string) (string, error) {
	if strings.TrimSpace(explicitPath) != "" {
		path, err := resolveRegularFile(explicitPath)
		if err != nil {
			return "", fmt.Errorf("resolve Caddy executable: %w", err)
		}
		return path, nil
	}
	if path, err := exec.LookPath(caddyExecutableName()); err == nil {
		return filepath.Abs(path)
	}
	for _, candidate := range platformCaddyBinaryCandidates() {
		path, err := resolveRegularFile(candidate)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect Caddy executable candidate %q: %w", candidate, err)
		}
	}
	return "", nil
}

func caddyExecutableName() string {
	if runtime.GOOS == "windows" {
		return "caddy.exe"
	}
	return "caddy"
}

func platformCaddyfileCandidates() []string {
	if runtime.GOOS == "windows" {
		paths := []string{`C:\Caddy\Caddyfile`}
		if programData := strings.TrimSpace(os.Getenv("ProgramData")); programData != "" {
			paths = append(paths, filepath.Join(programData, "Caddy", "Caddyfile"))
		}
		if programFiles := strings.TrimSpace(os.Getenv("ProgramFiles")); programFiles != "" {
			paths = append(paths, filepath.Join(programFiles, "Caddy", "Caddyfile"))
		}
		return paths
	}
	return []string{
		"/etc/caddy/Caddyfile",
		"/usr/local/etc/caddy/Caddyfile",
		"/opt/homebrew/etc/Caddyfile",
		"/etc/Caddyfile",
	}
}

func platformCaddyBinaryCandidates() []string {
	if runtime.GOOS == "windows" {
		paths := []string{`C:\Caddy\caddy.exe`}
		if programFiles := strings.TrimSpace(os.Getenv("ProgramFiles")); programFiles != "" {
			paths = append(paths, filepath.Join(programFiles, "Caddy", "caddy.exe"))
		}
		return paths
	}
	return []string{"/usr/bin/caddy", "/usr/local/bin/caddy", "/opt/homebrew/bin/caddy"}
}

func resolveRegularFile(path string) (string, error) {
	absPath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		return "", err
	}
	directInfo, err := os.Lstat(absPath)
	if err != nil {
		return "", err
	}
	resolved, err := evalSymlinks(absPath, directInfo)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", resolved)
	}
	return resolved, nil
}

func evalSymlinks(path string, directInfo os.FileInfo) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission) && directInfo.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	return "", err
}
