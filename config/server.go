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
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"go.yaml.in/yaml/v3"
)

type ServerConfig struct {
	Host                 string       `json:"host" yaml:"host"`
	SslCertPath          string       `json:"ssl_cert_path" yaml:"ssl_cert_path"`
	SslKeyPath           string       `json:"ssl_key_path" yaml:"ssl_key_path"`
	Domain               string       `json:"domain" yaml:"domain"`
	CdnIpHeader          string       `json:"cdn_ip_header" yaml:"cdn_ip_header"`
	TrustedProxies       []string     `json:"trusted_proxies" yaml:"trusted_proxies"`
	ParsedTrustedProxies []*net.IPNet `json:"-" yaml:"-"`
	FileCacheSizeMb      uint32       `json:"file_cache_size_mb" yaml:"file_cache_size_mb"`
	MaxActiveRequests    uint32       `json:"max_active_requests" yaml:"max_active_requests"`
	Port                 uint16       `json:"port" yaml:"port"`
	SslEnabled           bool         `json:"ssl_enabled" yaml:"ssl_enabled"`
	EnableCompression    bool         `json:"enable_compression" yaml:"enable_compression"`
}

func (s *ServerConfig) setDefaults() {
	if s.Host == "" {
		s.Host = DefaultHost()
	}
	if s.Port == 0 {
		s.Port = DefaultPort()
	}
	if s.Domain == "" {
		s.Domain = DefaultDomain()
	}
	if s.FileCacheSizeMb == 0 {
		s.FileCacheSizeMb = 100
	}
	if s.MaxActiveRequests == 0 {
		s.MaxActiveRequests = 2000
	}
	if s.TrustedProxies == nil {
		s.TrustedProxies = DefaultTrustedProxies()
	}
	if s.CdnIpHeader == "" {
		s.CdnIpHeader = DefaultCdnIpHeader()
	}
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
	s.setDefaults()
	type alias ServerConfig
	aux := (*alias)(s)
	err := sonic.ConfigFastest.Unmarshal(data, aux)
	if err == nil {
		s.ParseTrustedProxies()
	}
	return err
}

func (s *ServerConfig) UnmarshalYAML(value *yaml.Node) error {
	s.setDefaults()
	type alias ServerConfig
	aux := (*alias)(s)
	err := value.Decode(aux)
	if err == nil {
		s.ParseTrustedProxies()
	}
	return err
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
		Domain:            strings.Clone(s.Domain),
		EnableCompression: s.EnableCompression,
		FileCacheSizeMb:   s.FileCacheSizeMb,
		MaxActiveRequests: s.MaxActiveRequests,
		CdnIpHeader:       strings.Clone(s.CdnIpHeader),
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
