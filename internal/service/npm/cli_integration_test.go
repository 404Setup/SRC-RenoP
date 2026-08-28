/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func runNPMCLI(t *testing.T, executable, directory, userConfig, cache string, arguments ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"NPM_CONFIG_USERCONFIG="+userConfig,
		"NPM_CONFIG_CACHE="+cache,
		"NPM_CONFIG_UPDATE_NOTIFIER=false",
		"NO_PROXY=127.0.0.1,localhost",
		"no_proxy=127.0.0.1,localhost",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("npm command timed out: %s", strings.Join(arguments, " "))
	}
	require.NoErrorf(t, err, "npm %s failed:\n%s", strings.Join(arguments, " "), output)
	return strings.TrimSpace(string(output))
}

// TestNPMCLIEndToEnd verifies the wire contract with the installed official npm client.
func TestNPMCLIEndToEnd(t *testing.T) {
	executable, err := exec.LookPath("npm")
	if err != nil && runtime.GOOS == "windows" {
		executable, err = exec.LookPath("npm.cmd")
	}
	if err != nil {
		t.Skip("npm executable is not available")
	}
	app, state, _ := setupNPMTestApp(t)
	const packageName = "@team/demo"
	_, err = state.GetDB().CreateNPMPackage("npm", packageName, "alice", false, time.Now().UnixMilli())
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()
	t.Cleanup(func() {
		require.NoError(t, app.Shutdown())
		select {
		case serveErr := <-serveErrors:
			if serveErr != nil && !strings.Contains(strings.ToLower(serveErr.Error()), "closed") {
				t.Errorf("npm test server stopped unexpectedly: %v", serveErr)
			}
		case <-time.After(5 * time.Second):
			t.Error("npm test server did not stop")
		}
	})

	registry := "http://" + listener.Addr().String() + "/npm/"
	workspace := t.TempDir()
	userConfig := filepath.Join(workspace, ".npmrc")
	cache := filepath.Join(workspace, "cache")
	authKey := "//" + listener.Addr().String() + "/npm/:_authToken=test-token"
	require.NoError(t, os.WriteFile(userConfig,
		[]byte("registry="+registry+"\n"+authKey+"\nalways-auth=true\n"), 0o600))
	publisher := filepath.Join(workspace, "publisher")
	require.NoError(t, os.MkdirAll(publisher, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(publisher, "package.json"), []byte(`{
		"name":"@team/demo",
		"version":"4.5.6",
		"description":"Official npm CLI integration package",
		"main":"index.js",
		"scripts":{}
	}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(publisher, "index.js"),
		[]byte("module.exports = 'renop-npm-integration';\n"), 0o600))

	runNPMCLI(t, executable, publisher, userConfig, cache,
		"ping", "--registry", registry, "--loglevel", "error")
	runNPMCLI(t, executable, publisher, userConfig, cache,
		"publish", "--registry", registry, "--ignore-scripts", "--access", "public", "--loglevel", "error")
	runNPMCLI(t, executable, publisher, userConfig, cache,
		"dist-tag", "add", packageName+"@4.5.6", "stable", "--registry", registry, "--loglevel", "error")
	stable := runNPMCLI(t, executable, publisher, userConfig, cache,
		"view", packageName, "dist-tags.stable", "--registry", registry, "--loglevel", "error")
	require.Equal(t, "4.5.6", stable)

	consumer := filepath.Join(workspace, "consumer")
	require.NoError(t, os.MkdirAll(consumer, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(consumer, "package.json"),
		[]byte(`{"name":"consumer","version":"1.0.0","private":true}`), 0o600))
	runNPMCLI(t, executable, consumer, userConfig, cache,
		"install", packageName+"@4.5.6", "--registry", registry, "--ignore-scripts", "--no-audit", "--no-fund", "--loglevel", "error")
	installed, err := os.ReadFile(filepath.Join(consumer, "node_modules", "@team", "demo", "index.js"))
	require.NoError(t, err)
	require.Contains(t, string(installed), "renop-npm-integration")
}
