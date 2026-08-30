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
	"fmt"
	"strings"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type Repository struct {
	Name string `json:"name" yaml:"name"`
	// Format selects the client protocol. Empty is treated as Maven for
	// backwards compatibility with existing repositories.yaml files.
	Format              string                `json:"format,omitempty" yaml:"format,omitempty"`
	Visibility          string                `json:"visibility" yaml:"visibility"`
	Mirrors             []Mirror              `json:"mirrors" yaml:"mirrors"`
	AllowRedeployment   bool                  `json:"allow_redeployment" yaml:"allow_redeployment"`
	RequireGPGSignature bool                  `json:"require_gpg_signature" yaml:"require_gpg_signature"`
	PublicationReview   string                `json:"publication_review,omitempty" yaml:"publication_review,omitempty"`
	DownloadStatistics  *bool                 `json:"download_statistics,omitempty" yaml:"download_statistics,omitempty"`
	S3                  *S3Config             `json:"s3,omitempty" yaml:"s3,omitempty"`
	MavenRestore        *MavenRestoreSettings `json:"maven_restore,omitempty" yaml:"maven_restore,omitempty"`
}

// MavenRestoreSettings preserves Maven-only policy while a repository uses the files engine.
type MavenRestoreSettings struct {
	Format              string `json:"format" yaml:"format"`
	AllowRedeployment   bool   `json:"allow_redeployment" yaml:"allow_redeployment"`
	RequireGPGSignature bool   `json:"require_gpg_signature" yaml:"require_gpg_signature"`
	PublicationReview   string `json:"publication_review,omitempty" yaml:"publication_review,omitempty"`
}

type repositorySerialization struct {
	Name                string                `json:"name" yaml:"name"`
	Format              string                `json:"format,omitempty" yaml:"format,omitempty"`
	Visibility          string                `json:"visibility" yaml:"visibility"`
	Mirrors             []Mirror              `json:"mirrors" yaml:"mirrors"`
	AllowRedeployment   *bool                 `json:"allow_redeployment,omitempty" yaml:"allow_redeployment,omitempty"`
	RequireGPGSignature *bool                 `json:"require_gpg_signature,omitempty" yaml:"require_gpg_signature,omitempty"`
	PublicationReview   string                `json:"publication_review,omitempty" yaml:"publication_review,omitempty"`
	DownloadStatistics  *bool                 `json:"download_statistics,omitempty" yaml:"download_statistics,omitempty"`
	S3                  *S3Config             `json:"s3,omitempty" yaml:"s3,omitempty"`
	MavenRestore        *MavenRestoreSettings `json:"maven_restore,omitempty" yaml:"maven_restore,omitempty"`
}

const (
	RepositoryFormatMaven        = "maven"
	RepositoryFormatMavenClassic = "maven-classic"
	RepositoryFormatFiles        = "files"
	RepositoryFormatCargo        = "cargo"
	RepositoryFormatDocker       = "docker"
	RepositoryFormatNPM          = "npm"

	PublicationReviewOff          = "off"
	PublicationReviewNewPackages  = "new_packages"
	PublicationReviewEveryVersion = "every_version"
)

// NormalizePublicationReviewPolicy returns one supported publication-review policy.
func NormalizePublicationReviewPolicy(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", PublicationReviewOff:
		return PublicationReviewOff, true
	case PublicationReviewNewPackages:
		return PublicationReviewNewPackages, true
	case PublicationReviewEveryVersion:
		return PublicationReviewEveryVersion, true
	default:
		return "", false
	}
}

// PublicationReviewPolicy returns the normalized review policy supported by this repository.
func (r *Repository) PublicationReviewPolicy() string {
	if r == nil || !r.SupportsPublicationReview() {
		return PublicationReviewOff
	}
	policy, valid := NormalizePublicationReviewPolicy(r.PublicationReview)
	if !valid {
		return PublicationReviewOff
	}
	return policy
}

// SupportsPublicationReview reports whether this repository engine has an atomic moderated publication path.
func (r *Repository) SupportsPublicationReview() bool {
	if r == nil {
		return false
	}
	switch r.NormalizedFormat() {
	case RepositoryFormatMaven, RepositoryFormatNPM:
		return true
	default:
		return false
	}
}

// NormalizedFormat returns the protocol name while preserving the historical
// empty value as Maven.
func (r *Repository) NormalizedFormat() string {
	if r == nil {
		return RepositoryFormatMaven
	}
	format := strings.ToLower(strings.TrimSpace(r.Format))
	if format == "" {
		return RepositoryFormatMaven
	}
	if format == RepositoryFormatMavenClassic {
		return RepositoryFormatMaven
	}
	return format
}

// ConfiguredFormat returns the persisted protocol or Maven layout variant.
func (r *Repository) ConfiguredFormat() string {
	if r == nil {
		return RepositoryFormatMaven
	}
	format := strings.ToLower(strings.TrimSpace(r.Format))
	if format == "" {
		return RepositoryFormatMaven
	}
	return format
}

// UsesModernMavenLayout reports whether a Maven repository uses the domain catalog UI.
func (r *Repository) UsesModernMavenLayout() bool {
	return r != nil && r.NormalizedFormat() == RepositoryFormatMaven &&
		r.ConfiguredFormat() != RepositoryFormatMavenClassic
}

// serialization returns only fields supported by the repository protocol.
// Maven layout variants keep publication policy, Cargo keeps artifact URL
// templates, Docker omits GPG policy, and file storage keeps replacement only.
func (r Repository) serialization() repositorySerialization {
	serialized := repositorySerialization{
		Name: r.Name, Format: r.ConfiguredFormat(), Visibility: r.Visibility, S3: r.S3,
		Mirrors: make([]Mirror, len(r.Mirrors)), MavenRestore: r.MavenRestore.DeepCopy(),
		DownloadStatistics: cloneRepositoryBool(r.DownloadStatistics), PublicationReview: r.PublicationReviewPolicy(),
	}
	for i := range r.Mirrors {
		serialized.Mirrors[i] = r.Mirrors[i].DeepCopy()
	}
	if r.NormalizedFormat() == RepositoryFormatCargo || r.NormalizedFormat() == RepositoryFormatDocker ||
		r.NormalizedFormat() == RepositoryFormatNPM ||
		r.NormalizedFormat() == RepositoryFormatFiles {
		if r.NormalizedFormat() != RepositoryFormatNPM {
			serialized.PublicationReview = ""
		}
		if r.NormalizedFormat() == RepositoryFormatDocker || r.NormalizedFormat() == RepositoryFormatFiles {
			serialized.AllowRedeployment = &r.AllowRedeployment
		}
		return serialized
	}
	serialized.AllowRedeployment = &r.AllowRedeployment
	serialized.RequireGPGSignature = &r.RequireGPGSignature
	for i := range serialized.Mirrors {
		serialized.Mirrors[i].ArtifactURL = ""
	}
	return serialized
}

// DownloadStatisticsEnabled reports whether successful package downloads are counted.
// Structured package repositories default to enabled, while unstructured files opt in.
func (r *Repository) DownloadStatisticsEnabled() bool {
	if r == nil {
		return false
	}
	if r.DownloadStatistics != nil {
		return *r.DownloadStatistics
	}
	return r.NormalizedFormat() != RepositoryFormatFiles
}

func cloneRepositoryBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// MarshalJSON emits protocol-specific repository configuration fields.
func (r Repository) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.serialization())
}

// MarshalYAML emits protocol-specific repository configuration fields.
func (r Repository) MarshalYAML() (any, error) {
	return r.serialization(), nil
}

// IsSupportedRepositoryFormat reports whether the repository protocol is implemented.
func IsSupportedRepositoryFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", RepositoryFormatMaven, RepositoryFormatMavenClassic, RepositoryFormatFiles,
		RepositoryFormatCargo, RepositoryFormatDocker, RepositoryFormatNPM:
		return true
	default:
		return false
	}
}

type MirroredRepositorySettings struct {
	Authorization             *MirrorCredentials `json:"authorization" yaml:"authorization"`
	Reference                 string             `json:"reference" yaml:"reference"`
	HTTPProxy                 string             `json:"http_proxy" yaml:"http_proxy"`
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

type storageProviderSettingsAlias StorageProviderSettings

type storageProviderSettingsWire struct {
	*storageProviderSettingsAlias
}

func (s *StorageProviderSettings) UnmarshalJSON(data []byte) error {
	aux := &storageProviderSettingsWire{
		storageProviderSettingsAlias: (*storageProviderSettingsAlias)(s),
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
	return []byte(`{"type":"FileSystem"}`), nil
}

type RepositorySettings struct {
	ID                string                       `json:"id" yaml:"id"`
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

func (m *MavenSettings) validatePublicationReviewPolicies() error {
	for name, repo := range m.Repositories {
		if repo == nil {
			continue
		}
		policy, valid := NormalizePublicationReviewPolicy(repo.PublicationReview)
		if !valid {
			return fmt.Errorf("repository %q has an invalid publication review policy", name)
		}
		if policy != PublicationReviewOff && !repo.SupportsPublicationReview() {
			return fmt.Errorf("repository %q does not support publication review", name)
		}
		if restore := repo.MavenRestore; restore != nil {
			if _, valid := NormalizePublicationReviewPolicy(restore.PublicationReview); !valid {
				return fmt.Errorf("repository %q has an invalid Maven restore review policy", name)
			}
		}
	}
	return nil
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
	for _, repo := range m.Repositories {
		if repo != nil && strings.TrimSpace(repo.Format) == "" {
			repo.Format = RepositoryFormatMaven
		}
		if repo != nil && repo.NormalizedFormat() == RepositoryFormatFiles {
			repo.AllowRedeployment = true
			repo.RequireGPGSignature = false
			repo.PublicationReview = PublicationReviewOff
			if repo.MavenRestore != nil {
				restoredFormat := strings.ToLower(strings.TrimSpace(repo.MavenRestore.Format))
				if restoredFormat != RepositoryFormatMaven && restoredFormat != RepositoryFormatMavenClassic {
					repo.MavenRestore = nil
				} else {
					repo.MavenRestore.Format = restoredFormat
				}
			}
		} else if repo != nil {
			repo.MavenRestore = nil
			policy, valid := NormalizePublicationReviewPolicy(repo.PublicationReview)
			if !valid || !repo.SupportsPublicationReview() {
				policy = PublicationReviewOff
			}
			repo.PublicationReview = policy
			if policy != PublicationReviewOff {
				repo.AllowRedeployment = false
			}
		}
	}
	delete(m.Repositories, "snapshot")
}

func (m *MavenSettings) UnmarshalJSON(data []byte) error {
	type alias MavenSettings
	aux := (*alias)(m)
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if err := m.validatePublicationReviewPolicies(); err != nil {
		return err
	}
	m.setDefaults()
	return nil
}

func (m *MavenSettings) UnmarshalYAML(value *yaml.Node) error {
	type alias MavenSettings
	aux := (*alias)(m)
	if err := value.Decode(aux); err != nil {
		return err
	}
	if err := m.validatePublicationReviewPolicies(); err != nil {
		return err
	}
	m.setDefaults()
	return nil
}

type S3Config struct {
	Endpoint          string `json:"endpoint" yaml:"endpoint"`
	Bucket            string `json:"bucket" yaml:"bucket"`
	Region            string `json:"region" yaml:"region"`
	AccessKeyID       string `json:"access_key_id" yaml:"access_key_id"`
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
		AccessKeyID:       strings.Clone(s.AccessKeyID),
		SecretAccessKey:   strings.Clone(s.SecretAccessKey),
		KeyPrefix:         strings.Clone(s.KeyPrefix),
		ForcePathStyle:    s.ForcePathStyle,
		RedirectDownloads: s.RedirectDownloads,
	}
}

// DeepCopy returns an independent Maven policy snapshot.
func (m *MavenRestoreSettings) DeepCopy() *MavenRestoreSettings {
	if m == nil {
		return nil
	}
	return &MavenRestoreSettings{
		Format: strings.Clone(m.Format), AllowRedeployment: m.AllowRedeployment,
		RequireGPGSignature: m.RequireGPGSignature, PublicationReview: strings.Clone(m.PublicationReview),
	}
}

func (r *Repository) DeepCopy() *Repository {
	if r == nil {
		return nil
	}
	cloned := &Repository{
		Name:                strings.Clone(r.Name),
		Format:              strings.Clone(r.Format),
		Visibility:          strings.Clone(r.Visibility),
		AllowRedeployment:   r.AllowRedeployment,
		RequireGPGSignature: r.RequireGPGSignature,
		PublicationReview:   strings.Clone(r.PublicationReview),
		DownloadStatistics:  cloneRepositoryBool(r.DownloadStatistics),
		S3:                  r.S3.DeepCopy(),
		MavenRestore:        r.MavenRestore.DeepCopy(),
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
