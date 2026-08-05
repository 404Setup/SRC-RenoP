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

func DefaultCacheTtl() uint64 {
	return 3600
}

func DefaultMirrorTimeout() uint64 {
	return 30
}

func DefaultAuthMethod() string {
	return "BASIC"
}

func DefaultAllowedExtensions() []string {
	return []string{
		".jar", ".war", ".aar", ".pom", ".xml", ".module",
		".md5", ".sha1", ".sha256", ".sha512", ".asc",
	}
}

func DefaultConnectTimeout() int32 {
	return 3
}

func DefaultReadTimeout() int32 {
	return 15
}

func DefaultVisibility() string {
	return "PUBLIC"
}

func DefaultStoragePolicy() string {
	return "PRIORITIZE_UPSTREAM_METADATA"
}

func DefaultHost() string {
	return "0.0.0.0"
}

func DefaultPort() uint16 {
	return 3000
}

func DefaultTrue() bool {
	return true
}

func DefaultFrontendId() string {
	return "base-repository"
}

func DefaultFrontendTitle() string {
	return "RenoP Repository"
}

func DefaultFrontendDescription() string {
	return "Public Maven repository hosted through the RenoP"
}

func DefaultOrganizationWebsite() string {
	return "https://renop.pkg.one/"
}

func DefaultOrganizationLogo() string {
	return "/svg/logo.svg"
}

func DefaultDomain() string {
	return "localhost"
}

func DefaultDomains() []string {
	return []string{DefaultDomain()}
}

func DefaultCorsOrigins() []string {
	return []string{}
}

func DefaultTrustedProxies() []string {
	// Loopback is always trusted in IsTrustedProxy; list extra reverse-proxy hops here
	// (e.g. Docker bridge or a remote Caddy). Defaults stay empty so configs stay minimal.
	return []string{}
}

func DefaultCdnIpHeader() string {
	return "X-Forwarded-For"
}

func DefaultMavenSettings() MavenSettings {
	return MavenSettings{
		Repositories: map[string]*Repository{
			"releases": {
				Name:              "releases",
				Visibility:        "PUBLIC",
				Mirrors:           []Mirror{},
				AllowRedeployment: false,
			},
			"snapshots": {
				Name:              "snapshots",
				Visibility:        "PUBLIC",
				Mirrors:           []Mirror{},
				AllowRedeployment: true,
			},
			"private": {
				Name:              "private",
				Visibility:        "PRIVATE",
				Mirrors:           []Mirror{},
				AllowRedeployment: false,
			},
		},
	}
}

func DefaultServerConfig() ServerConfig {
	sc := ServerConfig{
		Host:              "0.0.0.0",
		Port:              3000,
		SslEnabled:        false,
		SslCertPath:       "",
		SslKeyPath:        "",
		Domains:           DefaultDomains(),
		CorsOrigins:       DefaultCorsOrigins(),
		EnableCompression: false,
		FileCacheSizeMb:   16,
		MaxActiveRequests: 512,
		TrustedProxies:    []string{},
		CdnIpHeader:       DefaultCdnIpHeader(),
	}
	sc.ParseTrustedProxies()
	return sc
}

func DefaultFrontendConfig() FrontendConfig {
	return FrontendConfig{
		Id:                  DefaultFrontendId(),
		Title:               DefaultFrontendTitle(),
		Description:         DefaultFrontendDescription(),
		OrganizationWebsite: DefaultOrganizationWebsite(),
		OrganizationLogo:    DefaultOrganizationLogo(),
		BackgroundUrl:       "",
		IcpLicense:          "",
		CachedIndexHtml:     []byte{},
	}
}

func DefaultAuditLogConfig() AuditLogConfig {
	return AuditLogConfig{
		RetentionDays: 14,
		MaxRows:       10000,
	}
}

func DefaultConfig() Config {
	return Config{
		StoragePath:          "storage",
		EnableJavadocPreview: true,
		JavadocExtractPath:   "",
		MaxJavadocSizeMb:     256,
		Frontend:             DefaultFrontendConfig(),
		Maven:                DefaultMavenSettings(),
		Server:               DefaultServerConfig(),
		Updater:              DefaultUpdaterConfig(),
		Database:             DefaultDatabaseConfig(),
		AuditLog:             DefaultAuditLogConfig(),
	}
}
