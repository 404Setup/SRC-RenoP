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
	"bytes"
	"strings"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

const (
	// FrontendFontSystem uses the operating system user-interface font stack.
	FrontendFontSystem = "system"
	// FrontendFontInter prefers a locally installed Inter family.
	FrontendFontInter = "inter"
	// FrontendFontNotoSans prefers locally installed Noto Sans families.
	FrontendFontNotoSans = "noto_sans"
	// FrontendFontOpenSans prefers a locally installed Open Sans family.
	FrontendFontOpenSans = "open_sans"
	// FrontendFontSourceSans prefers a locally installed Source Sans family.
	FrontendFontSourceSans = "source_sans"
	// FrontendFontCustom loads one administrator-provided webfont URL asynchronously.
	FrontendFontCustom = "custom"
)

// NormalizeFrontendFontPreset returns a supported canonical font preset.
func NormalizeFrontendFontPreset(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "":
		return FrontendFontSystem, true
	case FrontendFontSystem, FrontendFontInter, FrontendFontNotoSans,
		FrontendFontOpenSans, FrontendFontSourceSans, FrontendFontCustom:
		return value, true
	default:
		return FrontendFontSystem, false
	}
}

type FrontendConfig struct {
	ID                   string `json:"id" yaml:"id"`
	Title                string `json:"title" yaml:"title"`
	Description          string `json:"description" yaml:"description"`
	OrganizationWebsite  string `json:"organization_website" yaml:"organization_website"`
	OrganizationLogo     string `json:"organization_logo" yaml:"organization_logo"`
	BackgroundURL        string `json:"background_url" yaml:"background_url"`
	IcpLicense           string `json:"icp_license" yaml:"icp_license"`
	PublicSecurityFiling string `json:"public_security_filing" yaml:"public_security_filing"`
	LegalNoticeURL       string `json:"legal_notice_url" yaml:"legal_notice_url"`
	FontPreset           string `json:"font_preset" yaml:"font_preset"`
	FontURL              string `json:"font_url" yaml:"font_url"`
	CachedIndexHTML      []byte `json:"-" yaml:"-"`
}

func (f *FrontendConfig) setDefaults() {
	if f.ID == "" {
		f.ID = DefaultFrontendID()
	}
	if f.Title == "" {
		f.Title = DefaultFrontendTitle()
	}
	if f.Description == "" {
		f.Description = DefaultFrontendDescription()
	}
	if f.OrganizationWebsite == "" {
		f.OrganizationWebsite = DefaultOrganizationWebsite()
	}
	if f.OrganizationLogo == "" {
		f.OrganizationLogo = DefaultOrganizationLogo()
	}
	f.normalizeFont()
}

func (f *FrontendConfig) normalizeFont() {
	if preset, valid := NormalizeFrontendFontPreset(f.FontPreset); valid {
		f.FontPreset = preset
	} else {
		f.FontPreset = FrontendFontSystem
	}
	f.FontURL = strings.TrimSpace(f.FontURL)
}

func (f *FrontendConfig) UnmarshalJSON(data []byte) error {
	f.setDefaults()
	type alias FrontendConfig
	aux := (*alias)(f)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	f.normalizeFont()
	return nil
}

func (f *FrontendConfig) UnmarshalYAML(value *yaml.Node) error {
	f.setDefaults()
	type alias FrontendConfig
	aux := (*alias)(f)
	if err := value.Decode(aux); err != nil {
		return err
	}
	f.normalizeFont()
	return nil
}

func (f *FrontendConfig) DeepCopy() FrontendConfig {
	cloned := FrontendConfig{
		ID:                   strings.Clone(f.ID),
		Title:                strings.Clone(f.Title),
		Description:          strings.Clone(f.Description),
		OrganizationWebsite:  strings.Clone(f.OrganizationWebsite),
		OrganizationLogo:     strings.Clone(f.OrganizationLogo),
		BackgroundURL:        strings.Clone(f.BackgroundURL),
		IcpLicense:           strings.Clone(f.IcpLicense),
		PublicSecurityFiling: strings.Clone(f.PublicSecurityFiling),
		LegalNoticeURL:       strings.Clone(f.LegalNoticeURL),
		FontPreset:           strings.Clone(f.FontPreset),
		FontURL:              strings.Clone(f.FontURL),
	}
	if f.CachedIndexHTML != nil {
		cloned.CachedIndexHTML = bytes.Clone(f.CachedIndexHTML)
	}
	return cloned
}
