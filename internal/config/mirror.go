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
	"encoding/base64"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type Mirror struct {
	Authorization *MirrorCredentials `json:"authorization" yaml:"authorization"`
	// ProxyMode is the canonical selector: empty inherits the global proxy,
	// "direct" bypasses it, and any other value names a global proxy.
	ProxyMode string `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	// Proxy is retained only to read legacy repositories.yaml files. It is not
	// serialized and is ignored when an API update is applied.
	Proxy          *MirrorProxy `json:"-" yaml:"-"`
	Name           string       `json:"name" yaml:"name"`
	Url            string       `json:"url" yaml:"url"`
	EnabledDate    string       `json:"enabled_date" yaml:"enabled_date"`
	AllowArtifacts []string     `json:"allow_artifacts,omitempty" yaml:"allow_artifacts,omitempty"`
	DenyArtifacts  []string     `json:"deny_artifacts,omitempty" yaml:"deny_artifacts,omitempty"`
	CacheTtlSecs   uint64       `json:"cache_ttl_secs" yaml:"cache_ttl_secs"`
	TimeoutSecs    uint64       `json:"timeout_secs" yaml:"timeout_secs"`
	Persist        bool         `json:"persist" yaml:"persist"`
	NegativeCache  bool         `json:"negative_cache" yaml:"negative_cache"`
}

type MirrorProxy struct {
	URL      string `json:"url" yaml:"url"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

func (p *MirrorProxy) DeepCopy() *MirrorProxy {
	if p == nil {
		return nil
	}
	return &MirrorProxy{
		URL:      strings.Clone(p.URL),
		Username: strings.Clone(p.Username),
		Password: strings.Clone(p.Password),
	}
}

// parseMavenGroupArtifact extracts groupId and artifactId from a Maven repository path.
// Layouts handled:
//   - group/artifact/version/file
//   - group/artifact/maven-metadata.xml
//   - group/artifact/version/maven-metadata.xml
func parseMavenGroupArtifact(path string) (group, artifactId string) {
	clean := strings.Trim(path, "/")
	if clean == "" {
		return "", ""
	}
	parts := strings.Split(clean, "/")
	file := parts[len(parts)-1]
	rest := parts[:len(parts)-1]
	if len(rest) == 0 {
		return "", ""
	}

	isMeta := file == "maven-metadata.xml" || strings.HasPrefix(file, "maven-metadata.xml.")
	if isMeta {
		if len(rest) >= 2 && looksLikeMavenVersion(rest[len(rest)-1]) {
			artifactId = rest[len(rest)-2]
			group = strings.Join(rest[:len(rest)-2], ".")
			return group, artifactId
		}
		artifactId = rest[len(rest)-1]
		if len(rest) > 1 {
			group = strings.Join(rest[:len(rest)-1], ".")
		}
		return group, artifactId
	}

	if len(rest) >= 2 {
		artifactId = rest[len(rest)-2]
		group = strings.Join(rest[:len(rest)-2], ".")
		return group, artifactId
	}
	return rest[0], ""
}

func looksLikeMavenVersion(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, ".") || strings.Contains(strings.ToUpper(s), "SNAPSHOT") {
		return true
	}
	return s[0] >= '0' && s[0] <= '9'
}

func (m *Mirror) IsArtifactAllowed(path string) (bool, string) {
	if m == nil {
		return true, ""
	}
	hasAllow := len(m.AllowArtifacts) > 0
	hasDeny := len(m.DenyArtifacts) > 0

	if !hasAllow && !hasDeny {
		return true, ""
	}
	if hasAllow && hasDeny {
		return false, "Both allow and deny rules enabled on mirror"
	}

	clean := strings.Trim(path, "/")
	group, artifactId := parseMavenGroupArtifact(path)

	ga := group
	if artifactId != "" {
		ga = group + ":" + artifactId
	}

	match := func(pattern string) bool {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return false
		}
		if pattern == group || pattern == ga {
			return true
		}
		if group != "" && strings.HasPrefix(group, pattern+".") {
			return true
		}
		return false
	}

	if hasAllow {
		if slices.ContainsFunc(m.AllowArtifacts, match) {
			return true, ""
		}
		return false, "Artifact blocked: Not in mirror allow list (" + clean + ")"
	}

	if hasDeny {
		if slices.ContainsFunc(m.DenyArtifacts, match) {
			return false, "Artifact blocked: In mirror deny list (" + clean + ")"
		}
		return true, ""
	}

	return true, ""
}

func (m *Mirror) setDefaults() {
	if !m.Persist && m.CacheTtlSecs == 0 && m.TimeoutSecs == 0 && !m.NegativeCache {
		m.Persist = DefaultTrue()
		m.NegativeCache = DefaultTrue()
	}
	if m.CacheTtlSecs == 0 {
		m.CacheTtlSecs = DefaultCacheTtl()
	}
	if m.TimeoutSecs == 0 {
		m.TimeoutSecs = DefaultMirrorTimeout()
	}
	if m.EnabledDate == "" {
		m.EnabledDate = time.Now().Format("2006-01-02")
	}
}

func (m *Mirror) UnmarshalJSON(data []byte) error {
	m.setDefaults()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	proxyRaw, hasProxy := fields["proxy"]
	delete(fields, "proxy")
	type alias Mirror
	aux := (*alias)(m)
	clean, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(clean, aux); err != nil {
		return err
	}
	m.ProxyMode = ""
	m.Proxy = nil
	if hasProxy && len(proxyRaw) > 0 && string(proxyRaw) != "null" {
		if err := json.Unmarshal(proxyRaw, &m.ProxyMode); err != nil {
			var legacy MirrorProxy
			if legacyErr := json.Unmarshal(proxyRaw, &legacy); legacyErr != nil {
				return err
			}
			m.Proxy = &legacy
		}
	}
	m.ProxyMode = strings.TrimSpace(m.ProxyMode)
	return nil
}

func (m *Mirror) UnmarshalYAML(value *yaml.Node) error {
	m.setDefaults()
	var fields map[string]yaml.Node
	if err := value.Decode(&fields); err != nil {
		return err
	}
	proxyNode, hasProxy := fields["proxy"]
	delete(fields, "proxy")
	clean := yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for key, node := range fields {
		keyNode := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		nodeCopy := node
		clean.Content = append(clean.Content, &keyNode, &nodeCopy)
	}
	type alias Mirror
	aux := (*alias)(m)
	if err := clean.Decode(aux); err != nil {
		return err
	}
	m.ProxyMode = ""
	m.Proxy = nil
	if hasProxy && proxyNode.Kind != 0 && proxyNode.Tag != "!!null" {
		if proxyNode.Kind == yaml.ScalarNode {
			if err := proxyNode.Decode(&m.ProxyMode); err != nil {
				return err
			}
		} else {
			var legacy MirrorProxy
			if err := proxyNode.Decode(&legacy); err != nil {
				return err
			}
			m.Proxy = &legacy
		}
	}
	m.ProxyMode = strings.TrimSpace(m.ProxyMode)
	return nil
}

type MirrorCredentials struct {
	Method   string `json:"method" yaml:"method"`
	Login    string `json:"login" yaml:"login"`
	Password string `json:"password" yaml:"password"`

	cachedHeader string    `json:"-" yaml:"-"`
	once         sync.Once `json:"-" yaml:"-"`
}

func (m *MirrorCredentials) Validate() error {
	if m == nil {
		return nil
	}
	method := strings.ToLower(strings.TrimSpace(m.Method))
	switch method {
	case "", "none", "basic", "username/password", "bearer", "token":
		return nil
	case "custom-header", "custom_header", "request-header", "header":
		name := strings.TrimSpace(m.Login)
		if name == "" {
			if len(m.Password) > 4096 || strings.ContainsAny(m.Password, "\r\n") {
				return errors.New("custom authentication token is invalid")
			}
			return nil
		}
		if len(name) > 256 || !validMirrorHeaderName(name) {
			return errors.New("custom authentication header name is invalid")
		}
		if len(m.Password) > 4096 || strings.ContainsAny(m.Password, "\r\n") {
			return errors.New("custom authentication token is invalid")
		}
		lowerName := strings.ToLower(name)
		switch lowerName {
		case "host", "content-length", "connection", "proxy-connection", "transfer-encoding", "upgrade":
			return errors.New("custom authentication header is not allowed")
		}
		return nil
	default:
		return errors.New("unsupported mirror authentication method")
	}
}

func validMirrorHeaderName(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			continue
		}
		return false
	}
	return true
}

func (m *MirrorCredentials) setDefaults() {
	if m.Method == "" {
		m.Method = DefaultAuthMethod()
	}
}

func (m *MirrorCredentials) UnmarshalJSON(data []byte) error {
	m.setDefaults()
	type alias MirrorCredentials
	aux := (*alias)(m)
	return json.Unmarshal(data, aux)
}

func (m *MirrorCredentials) UnmarshalYAML(value *yaml.Node) error {
	m.setDefaults()
	type alias MirrorCredentials
	aux := (*alias)(m)
	return value.Decode(aux)
}

func (m *MirrorCredentials) GetAuthHeader() string {
	m.once.Do(func() {
		method := strings.ToLower(strings.TrimSpace(m.Method))
		if method == "basic" || method == "username/password" {
			credentials := m.Login + ":" + m.Password
			encoded := base64.StdEncoding.EncodeToString(unsafeConvert.BytePointer(credentials))
			m.cachedHeader = "Basic " + encoded
		} else if method == "bearer" || method == "token" {
			token := m.Password
			if token == "" {
				token = m.Login
			}
			m.cachedHeader = "Bearer " + token
		}
	})
	return m.cachedHeader
}

// Apply sets the configured mirror authentication on an outbound request.
func (m *MirrorCredentials) Apply(req *http.Request) error {
	if m == nil || req == nil {
		return nil
	}
	if err := m.Validate(); err != nil {
		return err
	}
	method := strings.ToLower(strings.TrimSpace(m.Method))
	if method == "custom-header" || method == "custom_header" || method == "request-header" || method == "header" {
		name := strings.TrimSpace(m.Login)
		if name != "" {
			req.Header.Set(name, m.Password)
		}
		return nil
	}
	if header := m.GetAuthHeader(); header != "" {
		req.Header.Set("Authorization", header)
	}
	return nil
}

func (m *MirrorCredentials) DeepCopy() *MirrorCredentials {
	if m == nil {
		return nil
	}
	return &MirrorCredentials{
		Method:       strings.Clone(m.Method),
		Login:        strings.Clone(m.Login),
		Password:     strings.Clone(m.Password),
		cachedHeader: strings.Clone(m.cachedHeader),
	}
}

func (m *Mirror) DeepCopy() Mirror {
	cloned := Mirror{
		Name:          strings.Clone(m.Name),
		Url:           strings.Clone(m.Url),
		Persist:       m.Persist,
		CacheTtlSecs:  m.CacheTtlSecs,
		NegativeCache: m.NegativeCache,
		TimeoutSecs:   m.TimeoutSecs,
		Authorization: m.Authorization.DeepCopy(),
		ProxyMode:     strings.Clone(m.ProxyMode),
		Proxy:         m.Proxy.DeepCopy(),
		EnabledDate:   strings.Clone(m.EnabledDate),
	}
	if m.AllowArtifacts != nil {
		cloned.AllowArtifacts = make([]string, len(m.AllowArtifacts))
		for i, s := range m.AllowArtifacts {
			cloned.AllowArtifacts[i] = strings.Clone(s)
		}
	}
	if m.DenyArtifacts != nil {
		cloned.DenyArtifacts = make([]string, len(m.DenyArtifacts))
		for i, s := range m.DenyArtifacts {
			cloned.DenyArtifacts[i] = strings.Clone(s)
		}
	}
	return cloned
}
