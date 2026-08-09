/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package settings

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/javadocs"
	"renop/internal/service/storage"
	"renop/internal/utils"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

func GetDomains(c fiber.Ctx) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	return protohttp.Write(c, &pb.SettingsDomainsResponse{
		Domains: []string{"frontend", "server", "storage", "updater", "index"},
	})
}

func GetDomainSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	cfg := state.Inner.Config.Load()
	switch c.Params("name") {
	case "frontend":
		return protohttp.Write(c, pb.FromFrontendConfig(cfg.Frontend))
	case "server":
		return protohttp.Write(c, pb.FromServerConfig(cfg.Server, cfg.Database, cfg.AuditLog))
	case "storage":
		return protohttp.Write(c, pb.FromStorageConfig(cfg))
	case "updater":
		return protohttp.Write(c, pb.FromUpdaterConfig(cfg.Updater))
	case "index":
		return protohttp.Write(c, &pb.IndexDomainSettings{})
	default:
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
}

func UpdateDomainSettings(c fiber.Ctx, state *core.AppState) error {
	if !isManager(c) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	name := strings.Clone(c.Params("name"))
	bodyBytes := bytes.Clone(c.Body())

	var frontendMsg *pb.FrontendConfig
	var serverMsg *pb.ServerConfig
	var storageMsg *pb.StorageConfig
	var updaterMsg *pb.UpdaterConfig

	switch name {
	case "frontend":
		msg := &pb.FrontendConfig{}
		if err := proto.Unmarshal(bodyBytes, msg); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		if msg.BackgroundUrl != "" {
			if err := validateBackgroundUrl(msg.BackgroundUrl); err != nil {
				var fiberErr *fiber.Error
				if errors.As(err, &fiberErr) {
					return c.Status(fiberErr.Code).SendString(fiberErr.Message)
				}
				return c.Status(fiber.StatusBadRequest).SendString(err.Error())
			}
		}
		frontendMsg = msg

	case "server":
		msg := &pb.ServerConfig{}
		if err := proto.Unmarshal(bodyBytes, msg); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		if msg.Port == 0 || msg.Port > 65535 {
			return c.Status(fiber.StatusBadRequest).SendString("Port must be between 1 and 65535")
		}
		if msg.MaxActiveRequests == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Max active requests must be positive")
		}
		if msg.Database != nil {
			driver := strings.ToLower(strings.TrimSpace(msg.Database.Driver))
			if driver != "sqlite3" && driver != "sqlite" && driver != "mysql" {
				return c.Status(fiber.StatusBadRequest).SendString("Invalid database driver")
			}
			if strings.TrimSpace(msg.Database.Dsn) == "" {
				return c.Status(fiber.StatusBadRequest).SendString("Database DSN must not be empty")
			}
		}
		serverMsg = msg

	case "storage":
		msg := &pb.StorageConfig{}
		if err := proto.Unmarshal(bodyBytes, msg); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		if strings.TrimSpace(msg.StoragePath) == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Storage path must not be empty")
		}
		if msg.MaxJavadocSizeMb <= 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Max Javadocs size limit must be positive")
		}
		storageMsg = msg

	case "updater":
		msg := &pb.UpdaterConfig{}
		if err := proto.Unmarshal(bodyBytes, msg); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
		}
		if msg.Channel != "release" && msg.Channel != "nightly" {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid channel")
		}
		if msg.Mode != "manual" && msg.Mode != "auto_check" && msg.Mode != "auto_install" && msg.Mode != "safe_install" {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid mode")
		}
		updaterMsg = msg

	case "index":
		return c.Status(fiber.StatusNotFound).SendString("Not found")

	default:
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	var storagePathChanged bool
	var newStoragePath string

	err := state.Inner.FileIndex.UpdateMetadataCallback(func() error {
		oldConfig := state.Inner.Config.Load()
		newConfig := oldConfig.DeepCopy()

		switch name {
		case "frontend":
			pb.ApplyFrontendConfig(&newConfig.Frontend, frontendMsg)
			newConfig.Frontend = newConfig.Frontend.DeepCopy()
		case "server":
			pb.ApplyServerConfig(&newConfig.Server, &newConfig.Database, &newConfig.AuditLog, serverMsg)
			newConfig.Server = newConfig.Server.DeepCopy()
		case "storage":
			oldPath := oldConfig.StoragePath
			pb.ApplyStorageConfig(newConfig, storageMsg)
			newConfig.StoragePath = strings.Clone(newConfig.StoragePath)
			newConfig.JavadocExtractPath = strings.Clone(newConfig.JavadocExtractPath)
			if !sameStoragePath(oldPath, newConfig.StoragePath) {
				storagePathChanged = true
				newStoragePath = newConfig.StoragePath
			}
		case "updater":
			pb.ApplyUpdaterConfig(&newConfig.Updater, updaterMsg)
			newConfig.Updater = newConfig.Updater.DeepCopy()
		}

		yamlData, err := yaml.Marshal(newConfig)
		if err != nil {
			return err
		}

		configPath := os.Getenv("RENOP_CONFIG")
		if configPath == "" {
			configPath = "config.yaml"
		}
		tmpPath := configPath + ".tmp"
		if err := os.WriteFile(tmpPath, yamlData, 0644); err != nil {
			return err
		}
		if err := utils.SafeRename(tmpPath, configPath); err != nil {
			return err
		}

		state.Inner.Config.Store(newConfig)
		storage.InitS3(newConfig)
		javadocs.InitJavadocs(newConfig)
		return nil
	})

	if err != nil {
		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			return c.Status(fiberErr.Code).SendString(fiberErr.Message)
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if storagePathChanged {
		onStoragePathChanged(state, newStoragePath)
	}

	user, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user,
		Operator:   op,
		Action:     "SETTINGS_UPDATE",
		Details:    "Updated domain settings (" + name + ")",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return c.Status(fiber.StatusOK).SendString("")
}

var nonPublicAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isValidPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range nonPublicAddressRanges {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

var errInvalidBackgroundWebP = errors.New("background is not a WebP image")

func validateBackgroundWebP(r io.Reader, maxSize int64) error {
	const signatureSize = 12
	if maxSize < signatureSize {
		return utils.ErrResponseTooLarge
	}

	var signature [signatureSize]byte
	if _, err := io.ReadFull(r, signature[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return errInvalidBackgroundWebP
		}
		return err
	}
	if signature[0] != 'R' || signature[1] != 'I' || signature[2] != 'F' || signature[3] != 'F' ||
		signature[8] != 'W' || signature[9] != 'E' || signature[10] != 'B' || signature[11] != 'P' {
		return errInvalidBackgroundWebP
	}

	remaining := maxSize - signatureSize
	written, err := io.Copy(io.Discard, io.LimitReader(r, remaining+1))
	if err != nil {
		return err
	}
	if written > remaining {
		return utils.ErrResponseTooLarge
	}
	return nil
}

func validateBackgroundUrl(bgUrl string) error {
	if !strings.HasPrefix(bgUrl, "http://") && !strings.HasPrefix(bgUrl, "https://") {
		return fiber.NewError(fiber.StatusBadRequest, "URL must be http or https")
	}

	parsedUrl, err := url.Parse(bgUrl)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL")
	}
	if parsedUrl.User != nil {
		return fiber.NewError(fiber.StatusBadRequest, "URL must not contain credentials")
	}

	host := parsedUrl.Hostname()
	if host == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL host")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Could not resolve host")
	}

	var validIP net.IP
	for _, ip := range ips {
		if isValidPublicIP(ip) {
			validIP = append(net.IP(nil), ip...)
			break
		}
	}

	if validIP == nil {
		return fiber.NewError(fiber.StatusBadRequest, "URL points to an internal or private IP")
	}
	pinnedIP := validIP.String()

	const maxBackgroundSize = 5 * 1024 * 1024
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DisableCompression: true,
		DisableKeepAlives:  true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil || port == "" {
				if parsedUrl.Scheme == "https" {
					port = "443"
				} else {
					port = "80"
				}
			}
			return dialer.DialContext(ctx, "tcp", net.JoinHostPort(pinnedIP, port))
		},
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxResponseHeaderBytes: 256 << 10,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: transport,
	}

	req, err := http.NewRequest(http.MethodGet, bgUrl, nil)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL")
	}
	req.Close = true
	defer utils.ScheduleNetworkWorkingSetTrim()
	resp, err := client.Do(req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Failed to access background URL or returned non-success status")
	}
	defer utils.DiscardHTTPBody(resp.Body, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		return fiber.NewError(fiber.StatusBadRequest, "Failed to access background URL or returned non-success status")
	}
	if resp.ContentLength > maxBackgroundSize {
		return fiber.NewError(fiber.StatusBadRequest, "Background image exceeds 5 MiB")
	}

	err = validateBackgroundWebP(resp.Body, maxBackgroundSize)
	if err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return fiber.NewError(fiber.StatusBadRequest, "Background image exceeds 5 MiB")
		}
		if errors.Is(err, errInvalidBackgroundWebP) {
			return fiber.NewError(fiber.StatusBadRequest, "Background URL must be a valid WebP image")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to read chunk")
	}
	return nil
}
