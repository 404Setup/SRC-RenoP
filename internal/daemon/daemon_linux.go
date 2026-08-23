//go:build linux

/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

const (
	systemdUnitPath = "/etc/systemd/system/renop.service"
	openrcInitPath  = "/etc/init.d/renop"
)

// IsWindowsService always returns false on Linux.
func IsWindowsService() bool {
	return false
}

// RunWindowsService is a no-op on Linux.
func RunWindowsService(runFn func()) error {
	return nil
}

// Install installs RenoP as a system service under systemd or OpenRC.
func Install() error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges required to install service; please run: sudo ./renop --install")
	}

	exePath, err := ExecutablePath()
	if err != nil {
		return err
	}
	workDir := WorkingDir(exePath)

	if isSystemd() {
		return installSystemd(exePath, workDir)
	}
	if isOpenRC() {
		return installOpenRC(exePath, workDir)
	}

	return errors.New("no supported init system found (systemd or OpenRC required)")
}

// Uninstall stops and removes the RenoP system service.
func Uninstall() error {
	if os.Geteuid() != 0 {
		return errors.New("root privileges required to uninstall service; please run: sudo ./renop --uninstall")
	}

	if isSystemd() || fileExists(systemdUnitPath) {
		return uninstallSystemd()
	}
	if isOpenRC() || fileExists(openrcInitPath) {
		return uninstallOpenRC()
	}

	return errors.New("no installed RenoP service found")
}

func isSystemd() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func isOpenRC() bool {
	if _, err := os.Stat("/run/openrc"); err == nil {
		return true
	}
	_, err := exec.LookPath("rc-service")
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func installSystemd(exePath, workDir string) error {
	unitContent := fmt.Sprintf(`[Unit]
Description=RenoP Package Repository Server
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=always
RestartSec=5s
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`, workDir, exePath)

	if err := os.WriteFile(systemdUnitPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("write systemd unit file %s: %w", systemdUnitPath, err)
	}

	_ = exec.Command("systemctl", "daemon-reload").Run()
	if err := exec.Command("systemctl", "enable", "--now", "renop.service").Run(); err != nil {
		fmt.Printf("Service unit written to %s, but failed to enable/start automatically: %v\n", systemdUnitPath, err)
		return nil
	}

	fmt.Printf("RenoP systemd service installed and started successfully (%s).\n", systemdUnitPath)
	return nil
}

func uninstallSystemd() error {
	_ = exec.Command("systemctl", "disable", "--now", "renop.service").Run()
	if err := os.Remove(systemdUnitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove systemd unit file %s: %w", systemdUnitPath, err)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	fmt.Println("RenoP systemd service uninstalled successfully.")
	return nil
}

func installOpenRC(exePath, workDir string) error {
	initScript := fmt.Sprintf(`#!/sbin/openrc-run
description="RenoP Package Repository Server"
directory="%s"
command="%s"
command_background=true
pidfile="/run/renop.pid"
output_log="/var/log/renop.log"
error_log="/var/log/renop.err"

depend() {
    need net
    after firewall
}
`, workDir, exePath)

	if err := os.WriteFile(openrcInitPath, []byte(initScript), 0755); err != nil {
		return fmt.Errorf("write OpenRC init script %s: %w", openrcInitPath, err)
	}

	_ = exec.Command("rc-update", "add", "renop", "default").Run()
	_ = exec.Command("rc-service", "renop", "start").Run()

	fmt.Printf("RenoP OpenRC service installed and started successfully (%s).\n", openrcInitPath)
	return nil
}

func uninstallOpenRC() error {
	_ = exec.Command("rc-service", "renop", "stop").Run()
	_ = exec.Command("rc-update", "del", "renop", "default").Run()
	if err := os.Remove(openrcInitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove OpenRC init script %s: %w", openrcInitPath, err)
	}
	fmt.Println("RenoP OpenRC service uninstalled successfully.")
	return nil
}
