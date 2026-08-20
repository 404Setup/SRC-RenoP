/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
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
	"renop/internal/service/outboundproxy"
	"renop/internal/utils"
)

const maxOutboundProxyClients = 16

type outboundProxyKey struct {
	url      string
	username string
	password string
}

type cachedOutboundProxyClient struct {
	client   *http.Client
	lastUsed uint64
}

var outboundProxyClients = struct {
	sync.Mutex
	clients map[outboundProxyKey]cachedOutboundProxyClient
	tick    uint64
}{clients: make(map[outboundProxyKey]cachedOutboundProxyClient)}

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

func newOutboundProxyClient(proxyConfig *config.OutboundProxy) (*http.Client, error) {
	if proxyConfig == nil {
		return httpClient, nil
	}
	transport := utils.DefaultTransport.Clone()
	if err := outboundproxy.ConfigureTransport(transport, proxyConfig); err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport, CheckRedirect: checkMirrorRedirect}, nil
}

// clientForMirror resolves the mirror selector and returns a shared direct
// client or a bounded cached client for the selected named global proxy.
func clientForMirror(mirror *config.Mirror, proxyConfig config.ProxyConfig) (*http.Client, error) {
	selection := ""
	if mirror != nil {
		selection = mirror.ProxyMode
	}
	selected, err := outboundproxy.ResolveMirrorSelection(selection, proxyConfig)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return httpClient, nil
	}

	key := outboundProxyKey{
		url:      strings.TrimSpace(selected.URL),
		username: selected.Username,
		password: selected.Password,
	}

	outboundProxyClients.Lock()
	defer outboundProxyClients.Unlock()
	outboundProxyClients.tick++
	if cached, ok := outboundProxyClients.clients[key]; ok {
		cached.lastUsed = outboundProxyClients.tick
		outboundProxyClients.clients[key] = cached
		return cached.client, nil
	}

	client, err := newOutboundProxyClient(selected)
	if err != nil {
		return nil, err
	}
	if len(outboundProxyClients.clients) >= maxOutboundProxyClients {
		var oldestKey outboundProxyKey
		var oldest cachedOutboundProxyClient
		first := true
		for candidateKey, candidate := range outboundProxyClients.clients {
			if first || candidate.lastUsed < oldest.lastUsed {
				oldestKey = candidateKey
				oldest = candidate
				first = false
			}
		}
		delete(outboundProxyClients.clients, oldestKey)
		oldest.client.CloseIdleConnections()
	}
	outboundProxyClients.clients[key] = cachedOutboundProxyClient{client: client, lastUsed: outboundProxyClients.tick}
	return client, nil
}
