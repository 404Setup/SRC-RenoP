/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package outboundproxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	xproxy "golang.org/x/net/proxy"

	"renop/internal/config"
)

const MaxProxies = 16

func parse(proxyConfig *config.OutboundProxy) (*url.URL, error) {
	if proxyConfig == nil {
		return nil, nil
	}
	rawURL := strings.TrimSpace(proxyConfig.URL)
	if len(rawURL) > 2048 || len(proxyConfig.Name) > 64 || len(proxyConfig.Username) > 255 || len(proxyConfig.Password) > 255 {
		return nil, errors.New("global proxy configuration is too long")
	}
	if rawURL == "" || strings.IndexFunc(rawURL, unicode.IsSpace) >= 0 {
		return nil, errors.New("global proxy URL is invalid")
	}

	proxyURL, err := url.Parse(rawURL)
	if err != nil || proxyURL.Hostname() == "" {
		return nil, errors.New("global proxy URL is invalid")
	}
	if proxyURL.User != nil {
		return nil, errors.New("global proxy credentials must use the username and password fields")
	}
	if (proxyURL.Path != "" && proxyURL.Path != "/") || proxyURL.RawQuery != "" || proxyURL.Fragment != "" {
		return nil, errors.New("global proxy URL must not contain a path, query, or fragment")
	}

	proxyURL.Scheme = strings.ToLower(proxyURL.Scheme)
	proxyURL.Path = ""
	switch proxyURL.Scheme {
	case "http", "https":
	case "socks5":
		if proxyURL.Port() == "" {
			return nil, errors.New("SOCKS5 global proxy URL must include a port")
		}
	default:
		return nil, errors.New("global proxy URL must use http, https, or socks5")
	}
	return proxyURL, nil
}

// NormalizeConfig validates a global proxy list and returns a detached,
// canonical copy suitable for persistence.
func NormalizeConfig(proxyConfig config.ProxyConfig) (config.ProxyConfig, error) {
	if len(proxyConfig.Proxies) > MaxProxies {
		return config.ProxyConfig{}, fmt.Errorf("at most %d global proxies are allowed", MaxProxies)
	}

	normalized := config.ProxyConfig{
		Selected: strings.TrimSpace(proxyConfig.Selected),
		Proxies:  make([]config.OutboundProxy, 0, len(proxyConfig.Proxies)),
	}
	seen := make(map[string]struct{}, len(proxyConfig.Proxies))
	for i := range proxyConfig.Proxies {
		candidate := proxyConfig.Proxies[i].DeepCopy()
		candidate.Name = strings.TrimSpace(candidate.Name)
		if candidate.Name == "" {
			return config.ProxyConfig{}, errors.New("global proxy name is required")
		}
		if strings.IndexFunc(candidate.Name, unicode.IsControl) >= 0 {
			return config.ProxyConfig{}, errors.New("global proxy name is invalid")
		}
		key := strings.ToLower(candidate.Name)
		if _, exists := seen[key]; exists {
			return config.ProxyConfig{}, errors.New("global proxy names must be unique")
		}
		seen[key] = struct{}{}

		proxyURL, err := parse(&candidate)
		if err != nil {
			return config.ProxyConfig{}, err
		}
		candidate.URL = proxyURL.String()
		normalized.Proxies = append(normalized.Proxies, candidate)
	}

	if normalized.Selected != "" {
		selectedKey := strings.ToLower(normalized.Selected)
		for i := range normalized.Proxies {
			if strings.ToLower(normalized.Proxies[i].Name) == selectedKey {
				normalized.Selected = normalized.Proxies[i].Name
				return normalized, nil
			}
		}
		return config.ProxyConfig{}, errors.New("selected global proxy does not exist")
	}
	return normalized, nil
}

// Selected returns a validated detached copy of the configured global proxy,
// or nil when outbound requests should connect directly.
func Selected(proxyConfig config.ProxyConfig) (*config.OutboundProxy, error) {
	normalized, err := NormalizeConfig(proxyConfig)
	if err != nil {
		return nil, err
	}
	if normalized.Selected == "" {
		return nil, nil
	}
	for i := range normalized.Proxies {
		if normalized.Proxies[i].Name == normalized.Selected {
			selected := normalized.Proxies[i].DeepCopy()
			return &selected, nil
		}
	}
	return nil, errors.New("selected global proxy does not exist")
}

// ConfigureTransport routes a dedicated outbound HTTP transport through the
// supplied proxy. It does not alter the mirror proxy transport or its cache.
func ConfigureTransport(transport *http.Transport, proxyConfig *config.OutboundProxy) error {
	if transport == nil || proxyConfig == nil {
		return nil
	}
	proxyURL, err := parse(proxyConfig)
	if err != nil {
		return err
	}
	if proxyConfig.Username != "" || proxyConfig.Password != "" {
		proxyURL.User = url.UserPassword(proxyConfig.Username, proxyConfig.Password)
	}

	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
		transport.DialContext = (&net.Dialer{Timeout: 4 * time.Second, KeepAlive: -1}).DialContext
	case "socks5":
		var auth *xproxy.Auth
		if proxyConfig.Username != "" || proxyConfig.Password != "" {
			auth = &xproxy.Auth{User: proxyConfig.Username, Password: proxyConfig.Password}
		}
		dialer, err := xproxy.SOCKS5("tcp", proxyURL.Host, auth, &net.Dialer{Timeout: 4 * time.Second, KeepAlive: -1})
		if err != nil {
			return fmt.Errorf("create SOCKS5 global proxy: %w", err)
		}
		contextDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return errors.New("SOCKS5 global proxy does not support cancellation")
		}
		transport.Proxy = nil
		transport.DialContext = contextDialer.DialContext
	}
	return nil
}
