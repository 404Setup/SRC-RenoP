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
	"encoding/base64"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
)

const MaxJSONBodySize int64 = 1 << 20

// ReadJSONLimited decodes a JSON request without allowing an unknown-length
// stream to grow beyond the endpoint's control-plane body budget.
func ReadJSONLimited(c fiber.Ctx, dst any, maxSize int64) error {
	if maxSize <= 0 {
		return fiber.ErrRequestEntityTooLarge
	}
	req := c.Request()
	if contentLength := req.Header.ContentLength(); int64(contentLength) > maxSize {
		return fiber.ErrRequestEntityTooLarge
	}

	var body []byte
	if stream := req.BodyStream(); stream != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(stream, maxSize+1))
		if err != nil {
			return err
		}
		if int64(len(body)) > maxSize {
			return fiber.ErrRequestEntityTooLarge
		}
		req.SetBodyRaw(body)
	} else {
		body = req.Body()
		if int64(len(body)) > maxSize {
			return fiber.ErrRequestEntityTooLarge
		}
	}

	return json.Unmarshal(body, dst)
}

func ParseRange(rangeStr string, fileSize uint64) (uint64, uint64, bool) {
	rangeStr = strings.TrimSpace(rangeStr)
	if !strings.HasPrefix(rangeStr, "bytes=") {
		return 0, 0, false
	}

	ranges := rangeStr[6:]
	before, after, ok := strings.Cut(ranges, "-")
	if !ok {
		return 0, 0, false
	}

	startStr := strings.TrimSpace(before)
	endStr := strings.TrimSpace(after)

	if startStr == "" {
		suffixLen, err := strconv.ParseUint(endStr, 10, 64)
		if err != nil || suffixLen == 0 || fileSize == 0 {
			return 0, 0, false
		}
		start := uint64(0)
		if fileSize > suffixLen {
			start = fileSize - suffixLen
		}
		return start, fileSize - 1, true
	}
	start, err := strconv.ParseUint(startStr, 10, 64)
	if err != nil || start >= fileSize {
		return 0, 0, false
	}

	if endStr == "" {
		if fileSize == 0 {
			return start, 0, true
		}
		return start, fileSize - 1, true
	}
	end, err := strconv.ParseUint(endStr, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if end >= start {
		if end > fileSize-1 && fileSize > 0 {
			end = fileSize - 1
		} else if fileSize == 0 {
			end = 0
		}
		return start, end, true
	}

	return 0, 0, false
}

// ExtractIP returns the client IP for rate limiting and sessions.
func ExtractIP(c fiber.Ctx, serverConfig *config.ServerConfig) string {
	rawIP := peerIP(c)
	if serverConfig.CdnIpHeader == "" || !serverConfig.IsTrustedProxy(rawIP) {
		return rawIP
	}

	val := strings.TrimSpace(c.Get(serverConfig.CdnIpHeader))
	if val == "" {
		return rawIP
	}

	// Walk right-to-left: for X-Forwarded-For the rightmost entry is the nearest
	// hop. Skip IPs that are themselves trusted proxies; return the first real client.
	parts := strings.Split(val, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := normalizeForwardedIP(parts[i])
		if candidate == "" || net.ParseIP(candidate) == nil {
			continue
		}
		if serverConfig.IsTrustedProxy(candidate) {
			continue
		}
		return candidate
	}

	// Header only listed trusted hops (or was a single trusted IP). Prefer the
	// leftmost parseable entry when present; otherwise keep the socket peer.
	for _, part := range parts {
		candidate := normalizeForwardedIP(part)
		if candidate != "" && net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return rawIP
}

// peerIP is the TCP remote address (never derived from spoofable headers).
func peerIP(c fiber.Ctx) string {
	// With TrustProxy disabled (our default), c.IP() is fasthttp RemoteIP.
	return c.IP()
}

// normalizeForwardedIP trims space and optional surrounding quotes, and strips a
// trailing :port when present (some proxies append it).
func normalizeForwardedIP(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "[") {
		if end := strings.IndexByte(s, ']'); end != -1 {
			return s[1:end]
		}
		return ""
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host
	}
	return s
}

func DecodeB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
