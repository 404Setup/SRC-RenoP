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
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"

	"renop/config"
	"renop/core"
	"renop/javadocs"
	"renop/pb"
	"renop/storage"
	"renop/utils"
	"renop/utils/protohttp"
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

	cfg := state.Inner.Config.Load().(*config.Config)
	switch c.Params("name") {
	case "frontend":
		return protohttp.Write(c, pb.FromFrontendConfig(cfg.Frontend))
	case "server":
		return protohttp.Write(c, pb.FromServerConfig(cfg.Server))
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
		oldConfig := state.Inner.Config.Load().(*config.Config)
		newConfig := oldConfig.DeepCopy()

		switch name {
		case "frontend":
			pb.ApplyFrontendConfig(&newConfig.Frontend, frontendMsg)
			newConfig.Frontend = newConfig.Frontend.DeepCopy()
		case "server":
			pb.ApplyServerConfig(&newConfig.Server, serverMsg)
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

	return c.Status(fiber.StatusOK).SendString("")
}

func isValidPublicIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
		return false
	}
	return true
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

	validIp := ""
	for _, ip := range ips {
		if isValidPublicIP(ip.String()) {
			validIp = ip.String()
			break
		}
	}

	if validIp == "" {
		return fiber.NewError(fiber.StatusBadRequest, "URL points to an internal or private IP")
	}

	const maxBackgroundSize = 5 * 1024 * 1024
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(addr)
				if err != nil || port == "" {
					if parsedUrl.Scheme == "https" {
						port = "443"
					} else {
						port = "80"
					}
				}
				var d net.Dialer
				d.Timeout = 10 * time.Second
				return d.DialContext(ctx, "tcp", net.JoinHostPort(validIp, port))
			},
			ForceAttemptHTTP2:     false,
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
		},
	}

	req, err := http.NewRequest(http.MethodGet, bgUrl, nil)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL")
	}
	req.Close = true
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

	imgData, err := utils.ReadAllLimited(resp.Body, maxBackgroundSize)
	if err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return fiber.NewError(fiber.StatusBadRequest, "Background image exceeds 5 MiB")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to read chunk")
	}

	isWebpMagic := len(imgData) >= 12 &&
		unsafeConvert.StringPointer(imgData[0:4]) == "RIFF" &&
		unsafeConvert.StringPointer(imgData[8:12]) == "WEBP"
	if !isWebpMagic {
		return fiber.NewError(fiber.StatusBadRequest, "Background URL must be a valid WebP image")
	}
	return nil
}
