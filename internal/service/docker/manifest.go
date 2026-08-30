/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/goccy/go-json"
)

// MaxManifestSize bounds local, mirrored, and persisted OCI manifest JSON.
const MaxManifestSize = 4 << 20

// ErrManifestTooLarge indicates that manifest JSON exceeds MaxManifestSize.
var ErrManifestTooLarge = errors.New("manifest exceeds the size limit")

// ErrManifestDigestMismatch indicates that manifest JSON does not match its declared digest.
var ErrManifestDigestMismatch = errors.New("manifest digest does not match its content")

// ParsedManifest holds extracted metadata and raw payload of an OCI or Docker v2 manifest.
type ParsedManifest struct {
	Digest       string       `json:"digest"`
	MediaType    string       `json:"media_type"`
	ConfigDigest string       `json:"config_digest"`
	ConfigSize   int64        `json:"config_size"`
	Size         int64        `json:"size"`
	Layers       []Descriptor `json:"layers,omitempty"`
	Manifests    []Descriptor `json:"manifests,omitempty"`
	IsIndex      bool         `json:"is_index"`
	RawJSON      []byte       `json:"-"`
}

// CalculateDigest computes the canonical sha256:hex digest of a byte slice.
func CalculateDigest(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// ParseManifest parses and validates raw manifest JSON.
func ParseManifest(data []byte, headerContentType string) (*ParsedManifest, error) {
	if len(data) == 0 {
		return nil, errors.New("empty manifest body")
	}
	if len(data) > MaxManifestSize {
		return nil, ErrManifestTooLarge
	}

	digest := CalculateDigest(data)
	size := int64(len(data))

	var rawMap map[string]any
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, errors.New("invalid manifest JSON")
	}

	mediaType, _ := rawMap["mediaType"].(string)
	if mediaType == "" {
		mediaType = strings.TrimSpace(headerContentType)
	}

	if _, hasManifests := rawMap["manifests"]; hasManifests ||
		mediaType == MediaTypeDockerManifestList ||
		mediaType == MediaTypeOCIIndex1 {
		var index ManifestIndex
		if err := json.Unmarshal(data, &index); err != nil {
			return nil, err
		}
		if mediaType == "" {
			mediaType = MediaTypeDockerManifestList
		}
		return &ParsedManifest{
			Digest:    digest,
			MediaType: mediaType,
			Size:      size,
			Manifests: index.Manifests,
			IsIndex:   true,
			RawJSON:   data,
		}, nil
	}

	var single ManifestSchema2
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, err
	}

	if mediaType == "" {
		if single.MediaType != "" {
			mediaType = single.MediaType
		} else {
			mediaType = MediaTypeDockerManifest2
		}
	}

	configDigest := single.Config.Digest

	return &ParsedManifest{
		Digest:       digest,
		MediaType:    mediaType,
		ConfigDigest: configDigest,
		ConfigSize:   single.Config.Size,
		Size:         size,
		Layers:       single.Layers,
		IsIndex:      false,
		RawJSON:      data,
	}, nil
}
