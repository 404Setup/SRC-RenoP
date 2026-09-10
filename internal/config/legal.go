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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

// MaxLegalDocumentBytes bounds each administrator-authored Markdown document.
const MaxLegalDocumentBytes = 512 << 10

// LegalConfig owns the instance's legal documents and cookie notice.
type LegalConfig struct {
	PrivacyPolicy  string `json:"privacy_policy" yaml:"privacy_policy"`
	TermsOfService string `json:"terms_of_service" yaml:"terms_of_service"`
	LegalNotice    string `json:"legal_notice" yaml:"legal_notice"`
	CookieBanner   bool   `json:"cookie_banner" yaml:"cookie_banner"`
	revision       string
}

// DefaultLegalConfig supplies placeholders for the instance operator to replace.
func DefaultLegalConfig() LegalConfig {
	value := LegalConfig{
		PrivacyPolicy:  "Placeholder: the instance operator must describe the data collected, purposes, retention, third-party services, and privacy contact here.\n",
		TermsOfService: "Placeholder: the instance operator must describe account responsibilities, acceptable use, publication rules, and the applicable service terms here.\n",
		LegalNotice:    "Placeholder: the instance operator must provide the service operator's identity, contact details, and required legal disclosures here.\n",
		CookieBanner:   true,
	}
	value.updateRevision()
	return value
}

func (value *LegalConfig) updateRevision() {
	digest := sha256.Sum256([]byte(value.PrivacyPolicy + "\x00" + value.TermsOfService))
	value.revision = hex.EncodeToString(digest[:])
}

// Normalize validates content and restores placeholder documents when a field is empty.
func (value *LegalConfig) Normalize() error {
	defaults := DefaultLegalConfig()
	for _, entry := range []struct {
		content  *string
		fallback string
	}{
		{&value.PrivacyPolicy, defaults.PrivacyPolicy},
		{&value.TermsOfService, defaults.TermsOfService},
		{&value.LegalNotice, defaults.LegalNotice},
	} {
		if strings.TrimSpace(*entry.content) == "" {
			*entry.content = entry.fallback
		}
		if len(*entry.content) > MaxLegalDocumentBytes || !utf8.ValidString(*entry.content) || strings.ContainsRune(*entry.content, 0) {
			return errors.New("legal document must contain UTF-8 text within 512 KiB")
		}
	}
	value.updateRevision()
	return nil
}

// Revision identifies the privacy policy and terms accepted by a browser.
func (value LegalConfig) Revision() string {
	if value.revision == "" {
		value.updateRevision()
	}
	return value.revision
}

// DeepCopy returns an independently owned legal configuration snapshot.
func (value LegalConfig) DeepCopy() LegalConfig {
	value.PrivacyPolicy = strings.Clone(value.PrivacyPolicy)
	value.TermsOfService = strings.Clone(value.TermsOfService)
	value.LegalNotice = strings.Clone(value.LegalNotice)
	return value
}

func (value *LegalConfig) UnmarshalJSON(data []byte) error {
	*value = DefaultLegalConfig()
	type alias LegalConfig
	if err := json.Unmarshal(data, (*alias)(value)); err != nil {
		return err
	}
	return value.Normalize()
}

func (value *LegalConfig) UnmarshalYAML(node *yaml.Node) error {
	*value = DefaultLegalConfig()
	type alias LegalConfig
	if err := node.Decode((*alias)(value)); err != nil {
		return err
	}
	return value.Normalize()
}
