//go:build windows

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
	"fmt"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "renop"

type serviceHandler struct {
	runFn func()
}

func (h *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	go h.runFn()
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			changes <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		}
	}
	return false, 0
}

// IsWindowsService reports whether the process is running as a Windows Service.
func IsWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// RunWindowsService executes the service main loop under Windows SCM.
func RunWindowsService(runFn func()) error {
	return svc.Run(serviceName, &serviceHandler{runFn: runFn})
}

// Install installs RenoP as a Windows Service and starts it.
func Install() error {
	exePath, err := ExecutablePath()
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager: %w (ensure you are running as Administrator)", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service '%s' is already installed; run with --uninstall first to reinstall", serviceName)
	}

	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName: "RenoP Package Repository Server",
		Description: "High-performance self-hosted package repository server",
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("create service '%s': %w", serviceName, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		fmt.Printf("Service '%s' created successfully, but could not be started immediately: %v\n", serviceName, err)
		return nil
	}

	fmt.Printf("Service '%s' installed and started successfully.\n", serviceName)
	return nil
}

// Uninstall stops and removes the RenoP Windows Service.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager: %w (ensure you are running as Administrator)", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service '%s' is not installed: %w", serviceName, err)
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	time.Sleep(500 * time.Millisecond)

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service '%s': %w", serviceName, err)
	}

	fmt.Printf("Service '%s' stopped and uninstalled successfully.\n", serviceName)
	return nil
}
