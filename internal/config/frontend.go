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

type FrontendConfig struct {
	Id                   string `json:"id" yaml:"id"`
	Title                string `json:"title" yaml:"title"`
	Description          string `json:"description" yaml:"description"`
	OrganizationWebsite  string `json:"organization_website" yaml:"organization_website"`
	OrganizationLogo     string `json:"organization_logo" yaml:"organization_logo"`
	BackgroundUrl        string `json:"background_url" yaml:"background_url"`
	IcpLicense           string `json:"icp_license" yaml:"icp_license"`
	PublicSecurityFiling string `json:"public_security_filing" yaml:"public_security_filing"`
	LegalNoticeUrl       string `json:"legal_notice_url" yaml:"legal_notice_url"`
	CachedIndexHtml      []byte `json:"-" yaml:"-"`
}

func (f *FrontendConfig) setDefaults() {
	if f.Id == "" {
		f.Id = DefaultFrontendId()
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
}

func (f *FrontendConfig) UnmarshalJSON(data []byte) error {
	f.setDefaults()
	type alias FrontendConfig
	aux := (*alias)(f)
	return json.Unmarshal(data, aux)
}

func (f *FrontendConfig) UnmarshalYAML(value *yaml.Node) error {
	f.setDefaults()
	type alias FrontendConfig
	aux := (*alias)(f)
	return value.Decode(aux)
}

func (f *FrontendConfig) DeepCopy() FrontendConfig {
	cloned := FrontendConfig{
		Id:                   strings.Clone(f.Id),
		Title:                strings.Clone(f.Title),
		Description:          strings.Clone(f.Description),
		OrganizationWebsite:  strings.Clone(f.OrganizationWebsite),
		OrganizationLogo:     strings.Clone(f.OrganizationLogo),
		BackgroundUrl:        strings.Clone(f.BackgroundUrl),
		IcpLicense:           strings.Clone(f.IcpLicense),
		PublicSecurityFiling: strings.Clone(f.PublicSecurityFiling),
		LegalNoticeUrl:       strings.Clone(f.LegalNoticeUrl),
	}
	if f.CachedIndexHtml != nil {
		cloned.CachedIndexHtml = bytes.Clone(f.CachedIndexHtml)
	}
	return cloned
}
