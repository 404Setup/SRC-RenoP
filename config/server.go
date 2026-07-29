/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type ServerConfig struct {
	Host        string `json:"host" yaml:"host"`
	SslCertPath string `json:"ssl_cert_path" yaml:"ssl_cert_path"`
	SslKeyPath  string `json:"ssl_key_path" yaml:"ssl_key_path"`

	// Domains lists public hostnames for this instance (used by CORS defaults and UI/metadata).
	Domains []string `json:"domains" yaml:"domains"`

	// CorsOrigins adds browser CORS allow patterns on top of Domains.
	// Empty = only origins whose host matches Domains. "*" allows any origin.
	// Supports exact origins (https://a.example.com), hostnames, and wildcards
	// (*.pkg.one matches pkg.one and any subdomain). Domains always remain allowed.
	CorsOrigins []string `json:"cors_origins" yaml:"cors_origins"`

	CdnIpHeader          string       `json:"cdn_ip_header" yaml:"cdn_ip_header"`
	TrustedProxies       []string     `json:"trusted_proxies" yaml:"trusted_proxies"`
	ParsedTrustedProxies []*net.IPNet `json:"-" yaml:"-"`
	FileCacheSizeMb      uint32       `json:"file_cache_size_mb" yaml:"file_cache_size_mb"`
	MaxActiveRequests    uint32       `json:"max_active_requests" yaml:"max_active_requests"`
	Port                 uint16       `json:"port" yaml:"port"`
	SslEnabled           bool         `json:"ssl_enabled" yaml:"ssl_enabled"`
	EnableCompression    bool         `json:"enable_compression" yaml:"enable_compression"`
}

// serverConfigWire is used for JSON/YAML unmarshalling so we can accept the
// legacy singular "domain" key while serializing only "domains".
type serverConfigWire struct {
	Host              string   `json:"host" yaml:"host"`
	SslCertPath       string   `json:"ssl_cert_path" yaml:"ssl_cert_path"`
	SslKeyPath        string   `json:"ssl_key_path" yaml:"ssl_key_path"`
	Domain            string   `json:"domain" yaml:"domain"`
	Domains           []string `json:"domains" yaml:"domains"`
	CorsOrigins       []string `json:"cors_origins" yaml:"cors_origins"`
	CdnIpHeader       string   `json:"cdn_ip_header" yaml:"cdn_ip_header"`
	TrustedProxies    []string `json:"trusted_proxies" yaml:"trusted_proxies"`
	FileCacheSizeMb   uint32   `json:"file_cache_size_mb" yaml:"file_cache_size_mb"`
	MaxActiveRequests uint32   `json:"max_active_requests" yaml:"max_active_requests"`
	Port              uint16   `json:"port" yaml:"port"`
	SslEnabled        bool     `json:"ssl_enabled" yaml:"ssl_enabled"`
	EnableCompression bool     `json:"enable_compression" yaml:"enable_compression"`
}

func (s *ServerConfig) applyWire(w *serverConfigWire) {
	s.Host = w.Host
	s.SslCertPath = w.SslCertPath
	s.SslKeyPath = w.SslKeyPath
	// nil = key omitted (apply defaults later); non-nil (incl. empty) = explicit.
	if w.CorsOrigins != nil {
		s.CorsOrigins = normalizeOriginPatterns(w.CorsOrigins)
	}
	s.CdnIpHeader = w.CdnIpHeader
	s.TrustedProxies = w.TrustedProxies
	s.FileCacheSizeMb = w.FileCacheSizeMb
	s.MaxActiveRequests = w.MaxActiveRequests
	s.Port = w.Port
	s.SslEnabled = w.SslEnabled
	s.EnableCompression = w.EnableCompression

	if len(w.Domains) > 0 {
		s.Domains = normalizeDomainList(w.Domains)
	} else if strings.TrimSpace(w.Domain) != "" {
		// Legacy singular "domain" key.
		s.Domains = []string{strings.ToLower(strings.TrimSpace(w.Domain))}
	}
}

func (s *ServerConfig) setDefaults() {
	if s.Host == "" {
		s.Host = DefaultHost()
	}
	if s.Port == 0 {
		s.Port = DefaultPort()
	}
	if len(s.Domains) == 0 {
		s.Domains = DefaultDomains()
	} else {
		s.Domains = normalizeDomainList(s.Domains)
	}
	if s.CorsOrigins == nil {
		s.CorsOrigins = DefaultCorsOrigins()
	} else {
		s.CorsOrigins = normalizeOriginPatterns(s.CorsOrigins)
	}
	if s.FileCacheSizeMb == 0 {
		s.FileCacheSizeMb = 16
	}
	if s.MaxActiveRequests == 0 {
		s.MaxActiveRequests = 512
	}
	if s.TrustedProxies == nil {
		s.TrustedProxies = DefaultTrustedProxies()
	}
	if s.CdnIpHeader == "" {
		s.CdnIpHeader = DefaultCdnIpHeader()
	}
}

func normalizeDomainList(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		// Strip accidental scheme/path from domain entries.
		if strings.Contains(d, "://") {
			if u, err := url.Parse(d); err == nil && u.Hostname() != "" {
				d = strings.ToLower(u.Hostname())
			}
		}
		d = strings.TrimSuffix(d, ".")
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

func normalizeOriginPatterns(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p != "*" && !strings.Contains(p, "://") {
			p = strings.ToLower(p)
		} else if p != "*" {
			// Normalize host part of full origins to lowercase while keeping scheme.
			if u, err := url.Parse(p); err == nil && u.Scheme != "" && u.Host != "" {
				u.Scheme = strings.ToLower(u.Scheme)
				host := u.Hostname()
				port := u.Port()
				host = strings.ToLower(host)
				if port != "" {
					u.Host = net.JoinHostPort(host, port)
				} else {
					u.Host = host
				}
				u.Path = ""
				u.RawQuery = ""
				u.Fragment = ""
				p = u.String()
			}
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// NormalizePublicNames lowercases/dedupes domains and CORS patterns, and
// restores the default domain list when empty (after settings apply).
func (s *ServerConfig) NormalizePublicNames() {
	if s == nil {
		return
	}
	s.Domains = normalizeDomainList(s.Domains)
	if len(s.Domains) == 0 {
		s.Domains = DefaultDomains()
	}
	if s.CorsOrigins == nil {
		s.CorsOrigins = DefaultCorsOrigins()
	} else {
		s.CorsOrigins = normalizeOriginPatterns(s.CorsOrigins)
	}
}

// PrimaryDomain returns the first configured domain, or "localhost".
func (s *ServerConfig) PrimaryDomain() string {
	if s == nil || len(s.Domains) == 0 {
		return DefaultDomain()
	}
	return s.Domains[0]
}

// IsOriginAllowed reports whether a browser Origin may call this server under CORS.
//
// Policy:
//   - Instance Domains always form the base allowlist (any scheme/port).
//   - CorsOrigins adds extra patterns (exact origins, hostnames, wildcards).
//   - Pattern "*" in CorsOrigins allows every origin.
//   - Empty CorsOrigins means domains only.
//
// Full origins (https://app.example.com) must match exactly. Host wildcards like
// "*.pkg.one" match the apex and any subdomain.
func (s *ServerConfig) IsOriginAllowed(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	if s == nil {
		return false
	}

	// Explicit allow-all in cors_origins.
	for _, p := range s.CorsOrigins {
		if strings.TrimSpace(p) == "*" {
			return true
		}
	}

	// Base allowlist: configured public hostnames for this instance.
	for _, d := range s.Domains {
		if matchOriginPattern(origin, d) {
			return true
		}
	}

	// Additional patterns from cors_origins (union with domains, not replace).
	for _, p := range s.CorsOrigins {
		if matchOriginPattern(origin, p) {
			return true
		}
	}
	return false
}

func originHostname(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func matchOriginPattern(origin, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}

	// Full origin pattern (scheme://host[:port]).
	if strings.Contains(pattern, "://") {
		if strings.Contains(pattern, "*") {
			// e.g. https://*.pkg.one — match scheme and host wildcard.
			pu, err := url.Parse(pattern)
			if err != nil {
				return false
			}
			ou, err := url.Parse(origin)
			if err != nil {
				return false
			}
			if strings.ToLower(pu.Scheme) != strings.ToLower(ou.Scheme) {
				return false
			}
			if pu.Port() != "" && pu.Port() != ou.Port() {
				return false
			}
			return hostMatchesPattern(strings.ToLower(ou.Hostname()), strings.ToLower(pu.Hostname()))
		}
		// Exact origin compare (normalized).
		return normalizeExactOrigin(origin) == normalizeExactOrigin(pattern)
	}

	// Host-only or wildcard host pattern.
	return hostMatchesPattern(originHostname(origin), strings.ToLower(pattern))
}

func normalizeExactOrigin(o string) string {
	u, err := url.Parse(strings.TrimSpace(o))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimSpace(o)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String()
}

// hostMatchesPattern matches host against an exact hostname or "*.example.com".
// "*.pkg.one" matches "pkg.one" and any subdomain (e.g. "mvnc.pkg.one", "a.b.pkg.one").
func hostMatchesPattern(host, pattern string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if host == "" || pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		base := pattern[2:]
		if base == "" {
			return false
		}
		return host == base || strings.HasSuffix(host, "."+base)
	}
	return host == pattern
}

func (s *ServerConfig) ParseTrustedProxies() {
	s.ParsedTrustedProxies = []*net.IPNet{}
	for _, proxyCIDR := range s.TrustedProxies {
		if !strings.Contains(proxyCIDR, "/") {
			if ip := net.ParseIP(proxyCIDR); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				proxyCIDR = proxyCIDR + "/" + strconv.Itoa(bits)
			}
		}
		_, ipNet, err := net.ParseCIDR(proxyCIDR)
		if err == nil {
			s.ParsedTrustedProxies = append(s.ParsedTrustedProxies, ipNet)
		}
	}
}

func (s *ServerConfig) UnmarshalJSON(data []byte) error {
	var w serverConfigWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	s.applyWire(&w)
	s.setDefaults()
	s.ParseTrustedProxies()
	return nil
}

func (s *ServerConfig) UnmarshalYAML(value *yaml.Node) error {
	var w serverConfigWire
	if err := value.Decode(&w); err != nil {
		return err
	}
	s.applyWire(&w)
	s.setDefaults()
	s.ParseTrustedProxies()
	return nil
}

func (s *ServerConfig) IsTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, ipNet := range s.ParsedTrustedProxies {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *ServerConfig) DeepCopy() ServerConfig {
	cloned := ServerConfig{
		Host:              strings.Clone(s.Host),
		Port:              s.Port,
		SslEnabled:        s.SslEnabled,
		SslCertPath:       strings.Clone(s.SslCertPath),
		SslKeyPath:        strings.Clone(s.SslKeyPath),
		EnableCompression: s.EnableCompression,
		FileCacheSizeMb:   s.FileCacheSizeMb,
		MaxActiveRequests: s.MaxActiveRequests,
		CdnIpHeader:       strings.Clone(s.CdnIpHeader),
	}
	if s.Domains != nil {
		cloned.Domains = make([]string, len(s.Domains))
		for i, d := range s.Domains {
			cloned.Domains[i] = strings.Clone(d)
		}
	}
	if s.CorsOrigins != nil {
		cloned.CorsOrigins = make([]string, len(s.CorsOrigins))
		for i, o := range s.CorsOrigins {
			cloned.CorsOrigins[i] = strings.Clone(o)
		}
	}
	if s.TrustedProxies != nil {
		cloned.TrustedProxies = make([]string, len(s.TrustedProxies))
		for i, p := range s.TrustedProxies {
			cloned.TrustedProxies[i] = strings.Clone(p)
		}
	}
	if s.ParsedTrustedProxies != nil {
		cloned.ParsedTrustedProxies = make([]*net.IPNet, len(s.ParsedTrustedProxies))
		for i, ipNet := range s.ParsedTrustedProxies {
			if ipNet != nil {
				ipNetCopy := *ipNet
				if ipNet.IP != nil {
					ipNetCopy.IP = append(net.IP(nil), ipNet.IP...)
				}
				if ipNet.Mask != nil {
					ipNetCopy.Mask = append(net.IPMask(nil), ipNet.Mask...)
				}
				cloned.ParsedTrustedProxies[i] = &ipNetCopy
			}
		}
	}
	return cloned
}
