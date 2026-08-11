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
	"strings"

	"github.com/3JoB/unsafeConvert"
	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type Repository struct {
	Name              string    `json:"name" yaml:"name"`
	Visibility        string    `json:"visibility" yaml:"visibility"`
	Mirrors           []Mirror  `json:"mirrors" yaml:"mirrors"`
	AllowRedeployment bool      `json:"allow_redeployment" yaml:"allow_redeployment"`
	S3                *S3Config `json:"s3,omitempty" yaml:"s3,omitempty"`
}

type MirroredRepositorySettings struct {
	Authorization             *MirrorCredentials `json:"authorization" yaml:"authorization"`
	Reference                 string             `json:"reference" yaml:"reference"`
	HttpProxy                 string             `json:"http_proxy" yaml:"http_proxy"`
	AllowedGroups             []string           `json:"allowed_groups" yaml:"allowed_groups"`
	AllowedExtensions         []string           `json:"allowed_extensions" yaml:"allowed_extensions"`
	ConnectTimeout            int32              `json:"connect_timeout" yaml:"connect_timeout"`
	ReadTimeout               int32              `json:"read_timeout" yaml:"read_timeout"`
	Store                     bool               `json:"store" yaml:"store"`
	AuthenticatedFetchingOnly bool               `json:"authenticated_fetching_only" yaml:"authenticated_fetching_only"`
}

func (m *MirroredRepositorySettings) setDefaults() {
	if m.AllowedExtensions == nil {
		m.AllowedExtensions = DefaultAllowedExtensions()
	}
	if m.ConnectTimeout == 0 {
		m.ConnectTimeout = DefaultConnectTimeout()
	}
	if m.ReadTimeout == 0 {
		m.ReadTimeout = DefaultReadTimeout()
	}
}

func (m *MirroredRepositorySettings) UnmarshalJSON(data []byte) error {
	m.setDefaults()
	type alias MirroredRepositorySettings
	aux := (*alias)(m)
	return json.Unmarshal(data, aux)
}

func (m *MirroredRepositorySettings) UnmarshalYAML(value *yaml.Node) error {
	m.setDefaults()
	type alias MirroredRepositorySettings
	aux := (*alias)(m)
	return value.Decode(aux)
}

type StorageProviderSettings struct {
	Type  string          `json:"type" yaml:"type"`
	Value json.RawMessage `json:"-" yaml:"-"`
}

func (s *StorageProviderSettings) UnmarshalJSON(data []byte) error {
	type Alias StorageProviderSettings
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.Value = data
	return nil
}

func (s StorageProviderSettings) MarshalJSON() ([]byte, error) {
	if len(s.Value) > 0 {
		return s.Value, nil
	}
	return unsafeConvert.BytePointer(`{"type":"FileSystem"}`), nil
}

type RepositorySettings struct {
	Id                string                       `json:"id" yaml:"id"`
	Visibility        string                       `json:"visibility" yaml:"visibility"`
	Redeployment      bool                         `json:"redeployment" yaml:"redeployment"`
	PreserveSnapshots bool                         `json:"preserve_snapshots" yaml:"preserve_snapshots"`
	StorageProvider   StorageProviderSettings      `json:"storage_provider" yaml:"storage_provider"`
	StoragePolicy     string                       `json:"storage_policy" yaml:"storage_policy"`
	MetadataMaxAge    int64                        `json:"metadata_max_age" yaml:"metadata_max_age"`
	Proxied           []MirroredRepositorySettings `json:"proxied" yaml:"proxied"`
}

func (r *RepositorySettings) setDefaults() {
	if r.Visibility == "" {
		r.Visibility = DefaultVisibility()
	}
	if r.StoragePolicy == "" {
		r.StoragePolicy = DefaultStoragePolicy()
	}
}

func (r *RepositorySettings) UnmarshalJSON(data []byte) error {
	r.setDefaults()
	type alias RepositorySettings
	aux := (*alias)(r)
	return json.Unmarshal(data, aux)
}

func (r *RepositorySettings) UnmarshalYAML(value *yaml.Node) error {
	r.setDefaults()
	type alias RepositorySettings
	aux := (*alias)(r)
	return value.Decode(aux)
}

type MavenSettings struct {
	Repositories map[string]*Repository `json:"repositories" yaml:"repositories"`
}

func (m *MavenSettings) setDefaults() {
	if m.Repositories == nil {
		m.Repositories = make(map[string]*Repository)
	}
	defaults := DefaultMavenSettings().Repositories
	for k, v := range defaults {
		if _, ok := m.Repositories[k]; !ok {
			m.Repositories[k] = v
		}
	}
	delete(m.Repositories, "snapshot")
}

func (m *MavenSettings) UnmarshalJSON(data []byte) error {
	m.setDefaults()
	type alias MavenSettings
	aux := (*alias)(m)
	return json.Unmarshal(data, aux)
}

func (m *MavenSettings) UnmarshalYAML(value *yaml.Node) error {
	m.setDefaults()
	type alias MavenSettings
	aux := (*alias)(m)
	return value.Decode(aux)
}

type S3Config struct {
	Endpoint          string `json:"endpoint" yaml:"endpoint"`
	Bucket            string `json:"bucket" yaml:"bucket"`
	Region            string `json:"region" yaml:"region"`
	AccessKeyId       string `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey   string `json:"secret_access_key" yaml:"secret_access_key"`
	KeyPrefix         string `json:"key_prefix" yaml:"key_prefix"`
	Enabled           bool   `json:"enabled" yaml:"enabled"`
	ForcePathStyle    bool   `json:"force_path_style" yaml:"force_path_style"`
	RedirectDownloads bool   `json:"redirect_downloads" yaml:"redirect_downloads"`
}

func (s *S3Config) DeepCopy() *S3Config {
	if s == nil {
		return nil
	}
	return &S3Config{
		Enabled:           s.Enabled,
		Endpoint:          strings.Clone(s.Endpoint),
		Bucket:            strings.Clone(s.Bucket),
		Region:            strings.Clone(s.Region),
		AccessKeyId:       strings.Clone(s.AccessKeyId),
		SecretAccessKey:   strings.Clone(s.SecretAccessKey),
		KeyPrefix:         strings.Clone(s.KeyPrefix),
		ForcePathStyle:    s.ForcePathStyle,
		RedirectDownloads: s.RedirectDownloads,
	}
}

func (r *Repository) DeepCopy() *Repository {
	if r == nil {
		return nil
	}
	cloned := &Repository{
		Name:              strings.Clone(r.Name),
		Visibility:        strings.Clone(r.Visibility),
		AllowRedeployment: r.AllowRedeployment,
		S3:                r.S3.DeepCopy(),
	}
	if r.Mirrors != nil {
		cloned.Mirrors = make([]Mirror, len(r.Mirrors))
		for i, m := range r.Mirrors {
			cloned.Mirrors[i] = m.DeepCopy()
		}
	}
	return cloned
}

func (m *MavenSettings) DeepCopy() MavenSettings {
	cloned := MavenSettings{}
	if m.Repositories != nil {
		cloned.Repositories = make(map[string]*Repository, len(m.Repositories))
		for k, v := range m.Repositories {
			cloned.Repositories[strings.Clone(k)] = v.DeepCopy()
		}
	}
	return cloned
}
