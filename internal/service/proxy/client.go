/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	xproxy "golang.org/x/net/proxy"

	"renop/internal/config"
	"renop/internal/utils"
)

const maxMirrorProxyClients = 32

type mirrorProxyKey struct {
	url      string
	username string
	password string
}

type cachedMirrorProxyClient struct {
	client   *http.Client
	lastUsed uint64
}

var mirrorProxyClients = struct {
	sync.Mutex
	clients map[mirrorProxyKey]cachedMirrorProxyClient
	tick    uint64
}{clients: make(map[mirrorProxyKey]cachedMirrorProxyClient)}

func parseMirrorProxy(proxyConfig *config.MirrorProxy) (*url.URL, error) {
	if proxyConfig == nil || strings.TrimSpace(proxyConfig.URL) == "" {
		return nil, nil
	}
	if len(proxyConfig.URL) > 2048 || len(proxyConfig.Username) > 256 || len(proxyConfig.Password) > 1024 {
		return nil, errors.New("mirror proxy configuration is too long")
	}

	proxyURL, err := url.Parse(strings.TrimSpace(proxyConfig.URL))
	if err != nil || proxyURL.Hostname() == "" {
		return nil, errors.New("mirror proxy URL is invalid")
	}
	if proxyURL.User != nil {
		return nil, errors.New("mirror proxy credentials must use the username and password fields")
	}
	if proxyURL.Path != "" || proxyURL.RawQuery != "" || proxyURL.Fragment != "" {
		return nil, errors.New("mirror proxy URL must not contain a path, query, or fragment")
	}

	proxyURL.Scheme = strings.ToLower(proxyURL.Scheme)
	switch proxyURL.Scheme {
	case "http", "https":
	case "socks5":
		if proxyURL.Port() == "" {
			return nil, errors.New("SOCKS5 mirror proxy URL must include a port")
		}
	default:
		return nil, errors.New("mirror proxy URL must use http, https, or socks5")
	}
	return proxyURL, nil
}

// ValidateMirrorProxy validates a per-mirror proxy without opening a connection.
func ValidateMirrorProxy(proxyConfig *config.MirrorProxy) error {
	_, err := parseMirrorProxy(proxyConfig)
	return err
}

func newMirrorProxyClient(proxyConfig *config.MirrorProxy) (*http.Client, error) {
	proxyURL, err := parseMirrorProxy(proxyConfig)
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return httpClient, nil
	}

	transport := utils.DefaultTransport.Clone()
	switch proxyURL.Scheme {
	case "http", "https":
		if proxyConfig.Username != "" || proxyConfig.Password != "" {
			proxyURL.User = url.UserPassword(proxyConfig.Username, proxyConfig.Password)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5":
		var auth *xproxy.Auth
		if proxyConfig.Username != "" || proxyConfig.Password != "" {
			auth = &xproxy.Auth{User: proxyConfig.Username, Password: proxyConfig.Password}
		}
		dialer, err := xproxy.SOCKS5("tcp", proxyURL.Host, auth, &net.Dialer{Timeout: 10 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("create SOCKS5 mirror proxy: %w", err)
		}
		contextDialer, ok := dialer.(xproxy.ContextDialer)
		if !ok {
			return nil, errors.New("SOCKS5 mirror proxy does not support cancellation")
		}
		transport.Proxy = nil
		transport.DialContext = contextDialer.DialContext
	}

	return &http.Client{Transport: transport, CheckRedirect: checkMirrorRedirect}, nil
}

func clientForMirror(mirror *config.Mirror) (*http.Client, error) {
	if mirror == nil || mirror.Proxy == nil || strings.TrimSpace(mirror.Proxy.URL) == "" {
		return httpClient, nil
	}

	key := mirrorProxyKey{
		url:      strings.TrimSpace(mirror.Proxy.URL),
		username: mirror.Proxy.Username,
		password: mirror.Proxy.Password,
	}

	mirrorProxyClients.Lock()
	defer mirrorProxyClients.Unlock()
	mirrorProxyClients.tick++
	if cached, ok := mirrorProxyClients.clients[key]; ok {
		cached.lastUsed = mirrorProxyClients.tick
		mirrorProxyClients.clients[key] = cached
		return cached.client, nil
	}

	client, err := newMirrorProxyClient(mirror.Proxy)
	if err != nil {
		return nil, err
	}
	if len(mirrorProxyClients.clients) >= maxMirrorProxyClients {
		var oldestKey mirrorProxyKey
		var oldest cachedMirrorProxyClient
		first := true
		for candidateKey, candidate := range mirrorProxyClients.clients {
			if first || candidate.lastUsed < oldest.lastUsed {
				oldestKey = candidateKey
				oldest = candidate
				first = false
			}
		}
		delete(mirrorProxyClients.clients, oldestKey)
		oldest.client.CloseIdleConnections()
	}
	mirrorProxyClients.clients[key] = cachedMirrorProxyClient{client: client, lastUsed: mirrorProxyClients.tick}
	return client, nil
}
