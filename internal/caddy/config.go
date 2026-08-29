/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package caddy

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/net/idna"

	"renop/internal/config"
)

const (
	managedBlockPrefix = "# BEGIN RenoP managed reverse proxy: "
	managedBlockSuffix = "# END RenoP managed reverse proxy: "
)

// NormalizeHostname validates and canonicalizes a public hostname for Caddy and RenoP.
func NormalizeHostname(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("hostname is required")
	}
	if strings.ContainsAny(value, "\x00\r\n\t {}#") {
		return "", errors.New("hostname contains unsupported characters")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", errors.New("hostname URL is invalid")
		}
		if parsed.Path != "" && parsed.Path != "/" {
			return "", errors.New("hostname must not include a path")
		}
		if parsed.Port() != "" {
			return "", errors.New("hostname must not include a port")
		}
		value = parsed.Hostname()
	} else if strings.ContainsAny(value, "/:@") {
		return "", errors.New("hostname must not include a path, port, or credentials")
	}
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	if value == "" || strings.HasPrefix(value, "*.") {
		return "", errors.New("a concrete hostname is required")
	}
	if ip := net.ParseIP(value); ip != nil {
		return strings.ToLower(ip.String()), nil
	}
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return "", fmt.Errorf("convert hostname to ASCII: %w", err)
	}
	if len(ascii) > 253 {
		return "", errors.New("hostname exceeds 253 characters")
	}
	for _, label := range strings.Split(ascii, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("hostname contains an invalid DNS label")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return "", errors.New("hostname contains an invalid DNS label")
			}
		}
	}
	return ascii, nil
}

// BuildCaddyfile inserts or replaces RenoP's managed reverse-proxy site block.
func BuildCaddyfile(original []byte, hostname string, port uint16) ([]byte, error) {
	hostname, err := NormalizeHostname(hostname)
	if err != nil {
		return nil, err
	}
	if port == 0 {
		return nil, errors.New("RenoP port must not be zero")
	}
	lineEnding := "\n"
	if bytes.Contains(original, []byte("\r\n")) {
		lineEnding = "\r\n"
	}
	normalized := strings.ReplaceAll(string(original), "\r\n", "\n")
	begin := managedBlockPrefix + hostname
	end := managedBlockSuffix + hostname
	block := strings.Join([]string{
		begin,
		hostname + " {",
		"    reverse_proxy " + net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))) + " {",
		"        flush_interval -1",
		"    }",
		"}",
		end,
	}, "\n")

	beginOffset, err := uniqueLineOffset(normalized, begin)
	if err != nil {
		return nil, err
	}
	endOffset, err := uniqueLineOffset(normalized, end)
	if err != nil {
		return nil, err
	}
	switch {
	case beginOffset < 0 && endOffset >= 0, beginOffset >= 0 && endOffset < 0:
		return nil, fmt.Errorf("managed Caddy block for %s is incomplete", hostname)
	case beginOffset >= 0:
		if endOffset <= beginOffset {
			return nil, fmt.Errorf("managed Caddy block for %s has invalid marker order", hostname)
		}
		endOffset += len(end)
		if endOffset < len(normalized) && normalized[endOffset] == '\n' {
			endOffset++
			block += "\n"
		}
		normalized = normalized[:beginOffset] + block + normalized[endOffset:]
	default:
		normalized = strings.TrimRight(normalized, "\n")
		if normalized != "" {
			normalized += "\n\n"
		}
		normalized += block + "\n"
	}
	if lineEnding == "\r\n" {
		normalized = strings.ReplaceAll(normalized, "\n", "\r\n")
	}
	return []byte(normalized), nil
}

// BuildRenoPConfig updates the listener and public-domain settings used behind Caddy.
func BuildRenoPConfig(original []byte, hostname string) ([]byte, uint16, error) {
	hostname, err := NormalizeHostname(hostname)
	if err != nil {
		return nil, 0, err
	}
	if len(bytes.TrimSpace(original)) == 0 {
		original, err = yaml.Marshal(config.DefaultConfig())
		if err != nil {
			return nil, 0, fmt.Errorf("marshal default RenoP configuration: %w", err)
		}
	}
	var current config.Config
	if err := yaml.Unmarshal(original, &current); err != nil {
		return nil, 0, fmt.Errorf("parse RenoP configuration: %w", err)
	}
	if current.Server.Port == 0 {
		current.Server.Port = config.DefaultPort()
	}
	domains := append([]string(nil), current.Server.Domains...)
	if len(domains) == 0 || (len(domains) == 1 && strings.EqualFold(domains[0], config.DefaultDomain())) {
		domains = []string{hostname}
	} else if !containsFold(domains, hostname) {
		domains = append(domains, hostname)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(original, &document); err != nil {
		return nil, 0, fmt.Errorf("parse RenoP configuration document: %w", err)
	}
	root, err := documentMapping(&document)
	if err != nil {
		return nil, 0, err
	}
	server := ensureMappingValue(root, "server")
	setScalarValue(server, "host", "127.0.0.1", "!!str")
	setScalarValue(server, "ssl_enabled", "false", "!!bool")
	setScalarValue(server, "ssl_cert_path", "", "!!str")
	setScalarValue(server, "ssl_key_path", "", "!!str")
	setSequenceValue(server, "domains", domains)

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, 0, fmt.Errorf("encode RenoP configuration: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, 0, fmt.Errorf("close RenoP configuration encoder: %w", err)
	}
	return output.Bytes(), current.Server.Port, nil
}

func uniqueLineOffset(content, marker string) (int, error) {
	offset := -1
	cursor := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		candidate := strings.TrimSuffix(line, "\n")
		if candidate == marker {
			if offset >= 0 {
				return -1, fmt.Errorf("managed Caddy marker %q occurs more than once", marker)
			}
			offset = cursor
		}
		cursor += len(line)
	}
	return offset, nil
}

func documentMapping(document *yaml.Node) (*yaml.Node, error) {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("RenoP configuration root must be a mapping")
	}
	return document.Content[0], nil
}

func ensureMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			value := mapping.Content[i+1]
			if value.Kind != yaml.MappingNode {
				value.Kind = yaml.MappingNode
				value.Tag = "!!map"
				value.Value = ""
				value.Content = nil
			}
			return value
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
	)
	return mapping.Content[len(mapping.Content)-1]
}

func setScalarValue(mapping *yaml.Node, key, value, tag string) {
	node := ensureValue(mapping, key)
	node.Kind = yaml.ScalarNode
	node.Tag = tag
	node.Value = value
	node.Content = nil
}

func setSequenceValue(mapping *yaml.Node, key string, values []string) {
	node := ensureValue(mapping, key)
	node.Kind = yaml.SequenceNode
	node.Tag = "!!seq"
	node.Value = ""
	node.Content = make([]*yaml.Node, 0, len(values))
	for _, value := range values {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
}

func ensureValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	value := &yaml.Node{}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
	return value
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(value), "."), target) {
			return true
		}
	}
	return false
}
