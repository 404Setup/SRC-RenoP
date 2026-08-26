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
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/cargodocs"
	"renop/internal/service/gpg"
	"renop/internal/service/javadocs"
	"renop/internal/service/outboundproxy"
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
		Domains: []string{"frontend", "server", "proxy", "storage", "updater", "index"},
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
	case "proxy":
		return protohttp.Write(c, pb.FromProxyConfig(cfg.Proxy))
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
	var frontendMsg *pb.FrontendConfig
	var serverMsg *pb.ServerConfig
	var proxyMsg *pb.ProxyConfig
	var storageMsg *pb.StorageConfig
	var updaterMsg *pb.UpdaterConfig
	readConfig := func(msg proto.Message) error {
		if err := protohttp.Read(c, msg); err != nil {
			if err == fiber.ErrRequestEntityTooLarge {
				return err
			}
			return fiber.NewError(fiber.StatusBadRequest, "Bad Request")
		}
		return nil
	}

	switch name {
	case "frontend":
		msg := &pb.FrontendConfig{}
		if err := readConfig(msg); err != nil {
			return err
		}
		if msg.BackgroundUrl != "" {
			if err := validateBackgroundURL(msg.BackgroundUrl); err != nil {
				if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
					return c.Status(fiberErr.Code).SendString(fiberErr.Message)
				}
				return c.Status(fiber.StatusBadRequest).SendString(err.Error())
			}
		}
		if msg.LegalNoticeUrl != "" {
			if err := validateExternalLinkURL(msg.LegalNoticeUrl); err != nil {
				if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
					return c.Status(fiberErr.Code).SendString(fiberErr.Message)
				}
				return c.Status(fiber.StatusBadRequest).SendString(err.Error())
			}
		}
		frontendMsg = msg

	case "server":
		msg := &pb.ServerConfig{}
		if err := readConfig(msg); err != nil {
			return err
		}
		if msg.Port == 0 || msg.Port > 65535 {
			return c.Status(fiber.StatusBadRequest).SendString("Port must be between 1 and 65535")
		}
		if msg.MaxActiveRequests == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Max active requests must be positive")
		}
		if msg.Database != nil {
			driver := strings.ToLower(strings.TrimSpace(msg.Database.Driver))
			if driver != "sqlite3" && driver != "sqlite" && driver != "mysql" && driver != "postgres" && driver != "postgresql" && driver != "pgx" && driver != "pg" {
				return c.Status(fiber.StatusBadRequest).SendString("Invalid database driver")
			}
			if strings.TrimSpace(msg.Database.Dsn) == "" {
				return c.Status(fiber.StatusBadRequest).SendString("Database DSN must not be empty")
			}
		}
		if msg.Gpg != nil {
			keyServers, err := gpg.ValidateKeyServers(msg.Gpg.KeyServers)
			if err != nil {
				return c.Status(fiber.StatusBadRequest).SendString(err.Error())
			}
			msg.Gpg.KeyServers = keyServers
		}
		serverMsg = msg

	case "proxy":
		msg := &pb.ProxyConfig{}
		if err := readConfig(msg); err != nil {
			return err
		}
		candidate := config.ProxyConfig{}
		pb.ApplyProxyConfig(&candidate, msg)
		normalized, err := outboundproxy.NormalizeConfig(candidate)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
		}
		proxyMsg = pb.FromProxyConfig(normalized)

	case "storage":
		msg := &pb.StorageConfig{}
		if err := readConfig(msg); err != nil {
			return err
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
		if err := readConfig(msg); err != nil {
			return err
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
			newConfig.GPG = newConfig.Server.GPG.DeepCopy()
		case "proxy":
			pb.ApplyProxyConfig(&newConfig.Proxy, proxyMsg)
			newConfig.Proxy = newConfig.Proxy.DeepCopy()
		case "storage":
			oldPath := oldConfig.StoragePath
			pb.ApplyStorageConfig(newConfig, storageMsg)
			newConfig.StoragePath = strings.Clone(newConfig.StoragePath)
			newConfig.JavadocExtractPath = strings.Clone(newConfig.JavadocExtractPath)
			if !sameStoragePath(oldPath, newConfig.StoragePath) {
				unlock, lockErr := storage.AcquireGPGStoragePathChange(state)
				if lockErr != nil {
					if errors.Is(lockErr, storage.ErrGPGStoragePathChange) {
						return fiber.NewError(fiber.StatusConflict, lockErr.Error())
					}
					return lockErr
				}
				defer unlock()
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
		if err := utils.WritePrivateFile(tmpPath, yamlData); err != nil {
			return err
		}
		if err := utils.SafeRename(tmpPath, configPath); err != nil {
			return err
		}

		state.Inner.Config.Store(newConfig)
		storage.InitS3(newConfig)
		javadocs.InitJavadocs(newConfig)
		cargodocs.InitCargodocs(newConfig)
		return nil
	})

	if err != nil {
		if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
			return c.Status(fiberErr.Code).SendString(fiberErr.Message)
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if storagePathChanged {
		onStoragePathChanged(state, newStoragePath)
	}

	user, op, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username:   user,
		Operator:   op,
		Action:     audit.ActionSettingsUpdate,
		Details:    "Updated domain settings (" + name + ")",
		AuthMethod: authMethod,
		SessionID:  sessionID,
		IP:         ip,
	})

	return c.Status(fiber.StatusOK).SendString("")
}

func isValidPublicIP(ip net.IP) bool {
	return utils.IsPublicIP(ip)
}

var errInvalidBackgroundWebP = errors.New("background is not a WebP image")

func validateExternalLinkURL(rawURL string) error {
	if strings.IndexFunc(rawURL, unicode.IsSpace) >= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL")
	}
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil || parsedURL.Host == "" {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL")
	}
	if !strings.EqualFold(parsedURL.Scheme, "http") && !strings.EqualFold(parsedURL.Scheme, "https") {
		return fiber.NewError(fiber.StatusBadRequest, "URL must be http or https")
	}
	if parsedURL.User != nil {
		return fiber.NewError(fiber.StatusBadRequest, "URL must not contain credentials")
	}
	return nil
}

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

func validateBackgroundURL(bgURL string) error {
	if !strings.HasPrefix(bgURL, "http://") && !strings.HasPrefix(bgURL, "https://") {
		return fiber.NewError(fiber.StatusBadRequest, "URL must be http or https")
	}

	parsedURL, err := url.Parse(bgURL)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL")
	}
	if parsedURL.User != nil {
		return fiber.NewError(fiber.StatusBadRequest, "URL must not contain credentials")
	}

	host := parsedURL.Hostname()
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
				if parsedURL.Scheme == "https" {
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

	req, err := http.NewRequest(http.MethodGet, bgURL, nil)
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
