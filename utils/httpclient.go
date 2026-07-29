/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

var DefaultTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout: 10 * time.Second,
	}).DialContext,
	TLSClientConfig: &tls.Config{
		ClientSessionCache: nil,
	},
	ForceAttemptHTTP2:     false,
	DisableCompression:    false,
	DisableKeepAlives:     true,
	MaxIdleConns:          0,
	MaxIdleConnsPerHost:   0,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 15 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// OutboundClient returns an *http.Client backed by DefaultTransport with the specified timeout.
func OutboundClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: DefaultTransport,
		Timeout:   timeout,
	}
}
