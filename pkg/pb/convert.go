/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package pb contains generated wire messages and domain conversion helpers.
package pb

import (
	"strings"
	"time"

	"renop/internal/config"
	"renop/internal/core"
)

func FromStatusSnapshots(snapshots []core.StatusSnapshot) *StatusSnapshotList {
	out := &StatusSnapshotList{
		Snapshots: make([]*StatusSnapshot, 0, len(snapshots)),
	}
	for i := range snapshots {
		s := snapshots[i]
		out.Snapshots = append(out.Snapshots, &StatusSnapshot{
			Timestamp:   s.Timestamp,
			UsedMemory:  s.UsedMemory,
			VssMemory:   s.VssMemory,
			UsedThreads: s.UsedThreads,
			OpenFiles:   s.OpenFiles,
		})
	}
	return out
}

func FromAccessTokenDto(t core.AccessTokenDto) *AccessTokenDto {
	msg := &AccessTokenDto{
		Identifier: &AccessTokenIdentifier{
			Type:  string(t.Identifier.Type),
			Value: t.Identifier.Value,
		},
		Name:        t.Name,
		CreatedAt:   t.CreatedAt,
		Description: t.Description,
		Tokens:      append([]string(nil), t.Tokens...),
		Permissions: append([]string(nil), t.Permissions...),
	}
	if t.ExpiresAt != nil {
		msg.ExpiresAt = t.ExpiresAt
	}
	return msg
}

func FromSessionDetails(d core.SessionDetails) *SessionDetails {
	perms := make([]*AccessTokenPermission, 0, len(d.Permissions))
	for _, p := range d.Permissions {
		perms = append(perms, &AccessTokenPermission{
			Identifier: p.Identifier,
			Shortcut:   p.Shortcut,
		})
	}
	routes := make([]*Route, 0, len(d.Routes))
	for _, r := range d.Routes {
		routes = append(routes, &Route{
			Path: r.Path,
			Permission: &RoutePermission{
				Identifier: r.Permission.Identifier,
				Shortcut:   r.Permission.Shortcut,
			},
		})
	}
	return &SessionDetails{
		AccessToken:  FromAccessTokenDto(d.AccessToken),
		Permissions:  perms,
		Routes:       routes,
		SessionToken: d.SessionToken,
	}
}

func FromAccessTokenList(tokens []core.AccessTokenDto) *AccessTokenList {
	out := &AccessTokenList{
		Tokens: make([]*AccessTokenDto, 0, len(tokens)),
	}
	for _, t := range tokens {
		out.Tokens = append(out.Tokens, FromAccessTokenDto(t))
	}
	return out
}

func FromSessionDto(s core.SessionDto) *SessionDto {
	lm := s.LoginMethod
	if lm == "" {
		lm = "password"
	}
	return &SessionDto{
		PublicId:    s.PublicID,
		Username:    s.Username,
		Ip:          s.IP,
		UserAgent:   s.UserAgent,
		CreatedAt:   s.CreatedAt,
		LastActive:  s.LastActive,
		ExpiresAt:   s.ExpiresAt,
		Current:     s.Current,
		LoginMethod: lm,
	}
}

func FromSessionList(sessions []core.SessionDto) *SessionList {
	out := &SessionList{
		Sessions: make([]*SessionDto, 0, len(sessions)),
	}
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, FromSessionDto(s))
	}
	return out
}

func StatusOkSuccess() *StatusOk {
	return &StatusOk{Status: "success"}
}

func FromMirrorCredentials(c *config.MirrorCredentials) *MirrorCredentials {
	if c == nil {
		return nil
	}
	return &MirrorCredentials{
		Method:   c.Method,
		Login:    c.Login,
		Password: c.Password,
	}
}

func FromMirrorProxy(p *config.MirrorProxy) *MirrorProxy {
	if p == nil {
		return nil
	}
	return &MirrorProxy{
		Url:      p.URL,
		Username: p.Username,
		Password: p.Password,
	}
}

func FromMirror(m config.Mirror) *Mirror {
	return &Mirror{
		Name:           m.Name,
		Url:            m.URL,
		ArtifactUrl:    m.ArtifactURL,
		Persist:        m.Persist,
		CacheTtlSecs:   m.CacheTTLSecs,
		NegativeCache:  m.NegativeCache,
		TimeoutSecs:    m.TimeoutSecs,
		Authorization:  FromMirrorCredentials(m.Authorization),
		EnabledDate:    m.EnabledDate,
		AllowArtifacts: append([]string(nil), m.AllowArtifacts...),
		DenyArtifacts:  append([]string(nil), m.DenyArtifacts...),
		Proxy:          m.ProxyMode,
	}
}

func FromS3Config(s *config.S3Config) *S3Config {
	if s == nil {
		return nil
	}
	return &S3Config{
		Enabled:           s.Enabled,
		Endpoint:          s.Endpoint,
		Bucket:            s.Bucket,
		Region:            s.Region,
		AccessKeyId:       s.AccessKeyID,
		SecretAccessKey:   s.SecretAccessKey,
		KeyPrefix:         s.KeyPrefix,
		ForcePathStyle:    s.ForcePathStyle,
		RedirectDownloads: s.RedirectDownloads,
	}
}

func FromRepository(r *config.Repository) *Repository {
	if r == nil {
		return nil
	}
	mirrors := make([]*Mirror, 0, len(r.Mirrors))
	for _, m := range r.Mirrors {
		mirrors = append(mirrors, FromMirror(m))
	}
	return &Repository{
		Name:                r.Name,
		Format:              r.ConfiguredFormat(),
		Visibility:          r.Visibility,
		Mirrors:             mirrors,
		AllowRedeployment:   r.AllowRedeployment,
		RequireGpgSignature: r.RequireGPGSignature,
		S3:                  FromS3Config(r.S3),
	}
}

func FromMavenRepositories(repos map[string]*config.Repository) *MavenRepositoriesResponse {
	out := &MavenRepositoriesResponse{
		Repositories: make(map[string]*Repository, len(repos)),
	}
	for k, r := range repos {
		out.Repositories[k] = FromRepository(r)
	}
	return out
}

func FromFrontendConfig(f config.FrontendConfig) *FrontendConfig {
	return &FrontendConfig{
		Id:                   f.ID,
		Title:                f.Title,
		Description:          f.Description,
		OrganizationWebsite:  f.OrganizationWebsite,
		OrganizationLogo:     f.OrganizationLogo,
		BackgroundUrl:        f.BackgroundURL,
		IcpLicense:           f.IcpLicense,
		PublicSecurityFiling: f.PublicSecurityFiling,
		LegalNoticeUrl:       f.LegalNoticeURL,
		FontPreset:           f.FontPreset,
		FontUrl:              f.FontURL,
	}
}

// ApplyFrontendConfig writes protobuf fields onto dst, preserving CachedIndexHTML.
func ApplyFrontendConfig(dst *config.FrontendConfig, src *FrontendConfig) {
	if dst == nil || src == nil {
		return
	}
	cached := dst.CachedIndexHTML
	dst.ID = src.Id
	dst.Title = src.Title
	dst.Description = src.Description
	dst.OrganizationWebsite = src.OrganizationWebsite
	dst.OrganizationLogo = src.OrganizationLogo
	dst.BackgroundURL = src.BackgroundUrl
	dst.IcpLicense = src.IcpLicense
	dst.PublicSecurityFiling = src.PublicSecurityFiling
	dst.LegalNoticeURL = src.LegalNoticeUrl
	dst.FontPreset = src.FontPreset
	dst.FontURL = src.FontUrl
	dst.CachedIndexHTML = cached
}

func FromAuditLogConfig(a config.AuditLogConfig) *AuditLogConfig {
	return &AuditLogConfig{
		RetentionDays: int32(a.RetentionDays),
		MaxRows:       int32(a.MaxRows),
	}
}

func FromServerConfig(s config.ServerConfig, d config.DatabaseConfig, a config.AuditLogConfig) *ServerConfig {
	return &ServerConfig{
		Host:              s.Host,
		Port:              uint32(s.Port),
		SslEnabled:        s.SslEnabled,
		SslCertPath:       s.SslCertPath,
		SslKeyPath:        s.SslKeyPath,
		Domains:           append([]string(nil), s.Domains...),
		EnableCompression: s.EnableCompression,
		FileCacheSizeMb:   s.FileCacheSizeMb,
		MaxActiveRequests: s.MaxActiveRequests,
		TrustedProxies:    append([]string(nil), s.TrustedProxies...),
		CdnIpHeader:       s.CdnIPHeader,
		CorsOrigins:       append([]string(nil), s.CorsOrigins...),
		DebugMode:         s.DebugMode,
		Database:          FromDatabaseConfig(d),
		AuditLog:          FromAuditLogConfig(a),
		Gpg:               FromGPGConfig(s.GPG),
	}
}

// ApplyServerConfig writes protobuf fields onto dst and re-parses trusted proxies.
func ApplyServerConfig(dstServer *config.ServerConfig, dstDB *config.DatabaseConfig, dstAudit *config.AuditLogConfig, src *ServerConfig) {
	if dstServer == nil || src == nil {
		return
	}
	port := min(src.Port, 0xFFFF)
	dstServer.Host = src.Host
	dstServer.Port = uint16(port)
	dstServer.SslEnabled = src.SslEnabled
	dstServer.SslCertPath = src.SslCertPath
	dstServer.SslKeyPath = src.SslKeyPath
	dstServer.Domains = append([]string(nil), src.Domains...)
	dstServer.EnableCompression = src.EnableCompression
	dstServer.FileCacheSizeMb = src.FileCacheSizeMb
	dstServer.MaxActiveRequests = src.MaxActiveRequests
	dstServer.TrustedProxies = append([]string(nil), src.TrustedProxies...)
	dstServer.CdnIPHeader = src.CdnIpHeader
	dstServer.CorsOrigins = append([]string(nil), src.CorsOrigins...)
	dstServer.DebugMode = src.DebugMode
	dstServer.NormalizePublicNames()
	dstServer.ParseTrustedProxies()
	if dstDB != nil && src.Database != nil {
		ApplyDatabaseConfig(dstDB, src.Database)
	}
	if dstAudit != nil && src.AuditLog != nil {
		if src.AuditLog.RetentionDays > 0 {
			dstAudit.RetentionDays = int(src.AuditLog.RetentionDays)
		}
		if src.AuditLog.MaxRows > 0 {
			dstAudit.MaxRows = int(src.AuditLog.MaxRows)
		}
	}
	if src.Gpg != nil {
		ApplyGPGConfig(&dstServer.GPG, src.Gpg)
	}
}

func FromStorageConfig(c *config.Config) *StorageConfig {
	if c == nil {
		return &StorageConfig{}
	}
	return &StorageConfig{
		StoragePath:           c.StoragePath,
		EnableJavadocPreview:  c.EnableJavadocPreview,
		JavadocExtractPath:    c.JavadocExtractPath,
		MaxJavadocSizeMb:      c.MaxJavadocSizeMb,
		EnableCargodocPreview: c.EnableCargodocPreview,
		CargodocExtractPath:   c.CargodocExtractPath,
		MaxCargodocSizeMb:     c.MaxCargodocSizeMb,
	}
}

// ApplyStorageConfig writes protobuf storage-domain fields onto dst.
func ApplyStorageConfig(dst *config.Config, src *StorageConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.StoragePath = src.StoragePath
	dst.EnableJavadocPreview = src.EnableJavadocPreview
	dst.JavadocExtractPath = src.JavadocExtractPath
	dst.MaxJavadocSizeMb = src.MaxJavadocSizeMb
	dst.EnableCargodocPreview = src.EnableCargodocPreview
	dst.CargodocExtractPath = src.CargodocExtractPath
	dst.MaxCargodocSizeMb = src.MaxCargodocSizeMb
}

func FromGPGConfig(g config.GPGConfig) *GpgConfig {
	return &GpgConfig{KeyServers: append([]string(nil), g.KeyServers...)}
}

func ApplyGPGConfig(dst *config.GPGConfig, src *GpgConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.KeyServers = append([]string(nil), src.KeyServers...)
}

func FromProxyConfig(p config.ProxyConfig) *ProxyConfig {
	proxies := make([]*OutboundProxy, 0, len(p.Proxies))
	for i := range p.Proxies {
		proxies = append(proxies, &OutboundProxy{
			Name:     p.Proxies[i].Name,
			Url:      p.Proxies[i].URL,
			Username: p.Proxies[i].Username,
			Password: p.Proxies[i].Password,
		})
	}
	return &ProxyConfig{Selected: p.Selected, Proxies: proxies}
}

func ApplyProxyConfig(dst *config.ProxyConfig, src *ProxyConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.Selected = src.Selected
	dst.Proxies = make([]config.OutboundProxy, 0, len(src.Proxies))
	for _, proxy := range src.Proxies {
		if proxy == nil {
			continue
		}
		dst.Proxies = append(dst.Proxies, config.OutboundProxy{
			Name:     proxy.Name,
			URL:      proxy.Url,
			Username: proxy.Username,
			Password: proxy.Password,
		})
	}
}

func FromUpdaterConfig(u config.UpdaterConfig) *UpdaterConfig {
	return &UpdaterConfig{
		Channel: u.Channel,
		Mode:    u.Mode,
	}
}

// ApplyUpdaterConfig writes protobuf fields onto dst.
func ApplyUpdaterConfig(dst *config.UpdaterConfig, src *UpdaterConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.Channel = src.Channel
	dst.Mode = src.Mode
}

func FromDatabaseConfig(d config.DatabaseConfig) *DatabaseConfig {
	return &DatabaseConfig{
		Driver:             d.Driver,
		Dsn:                d.Dsn,
		MaxOpenConns:       int32(d.MaxOpenConns),
		MaxIdleConns:       int32(d.MaxIdleConns),
		ConnMaxLifetimeSec: int32(d.ConnMaxLifetimeSec),
	}
}

// ApplyDatabaseConfig writes protobuf fields onto dst.
func ApplyDatabaseConfig(dst *config.DatabaseConfig, src *DatabaseConfig) {
	if dst == nil || src == nil {
		return
	}
	dst.Driver = src.Driver
	dst.Dsn = src.Dsn
	if src.MaxOpenConns > 0 {
		dst.MaxOpenConns = int(src.MaxOpenConns)
	}
	if src.MaxIdleConns > 0 {
		dst.MaxIdleConns = int(src.MaxIdleConns)
	}
	if src.ConnMaxLifetimeSec > 0 {
		dst.ConnMaxLifetimeSec = int(src.ConnMaxLifetimeSec)
	}
}

func ToMirrorCredentials(c *MirrorCredentials) *config.MirrorCredentials {
	if c == nil {
		return nil
	}
	return &config.MirrorCredentials{
		Method:   c.Method,
		Login:    c.Login,
		Password: c.Password,
	}
}

func ToMirrorProxy(p *MirrorProxy) *config.MirrorProxy {
	if p == nil {
		return nil
	}
	return &config.MirrorProxy{
		URL:      p.Url,
		Username: p.Username,
		Password: p.Password,
	}
}

func ToMirror(m *Mirror) config.Mirror {
	if m == nil {
		return config.Mirror{}
	}
	out := config.Mirror{
		Name:           m.Name,
		URL:            m.Url,
		ArtifactURL:    m.ArtifactUrl,
		Persist:        m.Persist,
		CacheTTLSecs:   m.CacheTtlSecs,
		NegativeCache:  m.NegativeCache,
		TimeoutSecs:    m.TimeoutSecs,
		Authorization:  ToMirrorCredentials(m.Authorization),
		EnabledDate:    m.EnabledDate,
		AllowArtifacts: append([]string(nil), m.AllowArtifacts...),
		DenyArtifacts:  append([]string(nil), m.DenyArtifacts...),
		ProxyMode:      m.Proxy,
	}
	// Zero TTL/timeout from proto3 defaults is unsafe; match YAML load defaults.
	if out.CacheTTLSecs == 0 {
		out.CacheTTLSecs = config.DefaultCacheTTL()
	}
	if out.TimeoutSecs == 0 {
		out.TimeoutSecs = config.DefaultMirrorTimeout()
	}
	if out.EnabledDate == "" {
		out.EnabledDate = time.Now().Format("2006-01-02")
	}
	return out
}

func ToS3Config(s *S3Config) *config.S3Config {
	if s == nil {
		return nil
	}
	return &config.S3Config{
		Enabled:           s.Enabled,
		Endpoint:          s.Endpoint,
		Bucket:            s.Bucket,
		Region:            s.Region,
		AccessKeyID:       s.AccessKeyId,
		SecretAccessKey:   s.SecretAccessKey,
		KeyPrefix:         s.KeyPrefix,
		ForcePathStyle:    s.ForcePathStyle,
		RedirectDownloads: s.RedirectDownloads,
	}
}

// ToRepository converts a protobuf Repository into a config.Repository (full replace).
func ToRepository(r *Repository) *config.Repository {
	if r == nil {
		return nil
	}
	mirrors := make([]config.Mirror, 0, len(r.Mirrors))
	for _, m := range r.Mirrors {
		if m == nil {
			continue
		}
		mirrors = append(mirrors, ToMirror(m))
	}
	return &config.Repository{
		Name:                r.Name,
		Format:              strings.ToLower(strings.TrimSpace(r.Format)),
		Visibility:          r.Visibility,
		Mirrors:             mirrors,
		AllowRedeployment:   r.AllowRedeployment,
		RequireGPGSignature: r.RequireGpgSignature,
		S3:                  ToS3Config(r.S3),
	}
}

// SessionStoreFormatVersion is written into SessionStore.format_version.
const SessionStoreFormatVersion uint32 = 1

// FromSessionDBDtos builds a SessionStore for on-disk persistence.
func FromSessionDBDtos(dtos []core.SessionDBDto) *SessionStore {
	store := &SessionStore{
		FormatVersion: SessionStoreFormatVersion,
		Sessions:      make([]*StoredSession, 0, len(dtos)),
	}
	for i := range dtos {
		d := &dtos[i]
		lm := d.LoginMethod
		if lm == "" {
			lm = "password"
		}
		store.Sessions = append(store.Sessions, &StoredSession{
			PublicId:     d.PublicID,
			SessionToken: d.SessionToken,
			Username:     d.Username,
			Ip:           d.IP,
			UserAgent:    d.UserAgent,
			CreatedAt:    d.CreatedAt,
			LastActive:   d.LastActive,
			LoginMethod:  lm,
		})
	}
	return store
}

// ToSessionDBDtos converts a SessionStore into core DTOs used at bootstrap.
func ToSessionDBDtos(store *SessionStore) []core.SessionDBDto {
	if store == nil || len(store.Sessions) == 0 {
		return []core.SessionDBDto{}
	}
	out := make([]core.SessionDBDto, 0, len(store.Sessions))
	for _, s := range store.Sessions {
		if s == nil {
			continue
		}
		lm := s.LoginMethod
		if lm == "" {
			lm = "password"
		}
		out = append(out, core.SessionDBDto{
			PublicID:     s.PublicId,
			SessionToken: s.SessionToken,
			Username:     s.Username,
			IP:           s.Ip,
			UserAgent:    s.UserAgent,
			CreatedAt:    s.CreatedAt,
			LastActive:   s.LastActive,
			LoginMethod:  lm,
		})
	}
	return out
}

func FromFidoDeviceDto(d core.FidoDeviceDto) *FidoDeviceDto {
	return &FidoDeviceDto{
		Id:        d.ID,
		Username:  d.Username,
		Name:      d.Name,
		CreatedAt: d.CreatedAt,
	}
}

func FromFidoDeviceList(devs []core.FidoDeviceDto) *FidoDeviceList {
	out := &FidoDeviceList{
		Devices: make([]*FidoDeviceDto, 0, len(devs)),
	}
	for _, d := range devs {
		out.Devices = append(out.Devices, FromFidoDeviceDto(d))
	}
	return out
}

func FromUserGPGKey(key *core.UserGPGKey) *GpgKeyDto {
	if key == nil {
		return nil
	}
	return &GpgKeyDto{
		Fingerprint:     key.Fingerprint,
		KeyId:           key.KeyID,
		PrimaryIdentity: key.PrimaryIdentity,
		KeyCreatedAt:    key.KeyCreatedAt,
		KeyExpiresAt:    key.KeyExpiresAt,
		AddedAt:         key.AddedAt,
		RefreshedAt:     key.FetchedAt,
	}
}

func FromUserGPGKeys(keys []*core.UserGPGKey) *GpgKeyList {
	out := &GpgKeyList{Keys: make([]*GpgKeyDto, 0, len(keys))}
	for _, key := range keys {
		if converted := FromUserGPGKey(key); converted != nil {
			out.Keys = append(out.Keys, converted)
		}
	}
	return out
}

func FromGPGSignature(signature *core.GPGSignature) *GpgSignatureDetails {
	if signature == nil {
		return nil
	}
	return &GpgSignatureDetails{
		Repository:         signature.Repository,
		ArtifactPath:       signature.ArtifactPath,
		Fingerprint:        signature.Fingerprint,
		KeyId:              signature.KeyID,
		PrimaryIdentity:    signature.PrimaryIdentity,
		Uploader:           signature.Uploader,
		SignatureCreatedAt: signature.SignatureCreatedAt,
		VerifiedAt:         signature.VerifiedAt,
		HashAlgorithm:      signature.HashAlgorithm,
		PublicKeyAlgorithm: signature.PublicKeyAlgorithm,
	}
}

func FromGPGRelease(release *core.GPGRelease) *GpgReleaseDto {
	if release == nil {
		return nil
	}
	return &GpgReleaseDto{
		Id:               release.ID,
		Repository:       release.Repository,
		ArtifactPath:     release.ArtifactPath,
		Status:           release.Status,
		FailureReason:    release.FailureReason,
		Signed:           release.SignatureStagingPath != "",
		RequireSignature: release.RequireSignature,
		CreatedAt:        release.CreatedAt,
		UpdatedAt:        release.UpdatedAt,
		CompletedAt:      release.CompletedAt,
	}
}

func FromGPGReleases(releases []*core.GPGRelease, total, limit, offset int) *GpgReleaseList {
	out := &GpgReleaseList{
		Releases: make([]*GpgReleaseDto, 0, len(releases)),
		Total:    int32(total),
		Limit:    int32(limit),
		Offset:   int32(offset),
	}
	for _, release := range releases {
		if converted := FromGPGRelease(release); converted != nil {
			out.Releases = append(out.Releases, converted)
		}
	}
	return out
}

func FromCreateAccessTokenResponse(res core.CreateAccessTokenResponse) *CreateAccessTokenResponse {
	return &CreateAccessTokenResponse{
		AccessToken: FromAccessTokenDto(res.AccessToken),
		Secret:      res.Secret,
	}
}

func FromAuditLogEntry(e *core.AuditLogEntry) *AuditLogEntryDto {
	if e == nil {
		return nil
	}
	return &AuditLogEntryDto{
		Id:         e.ID,
		Username:   e.Username,
		Operator:   e.Operator,
		Action:     e.Action,
		Details:    e.Details,
		AuthMethod: e.AuthMethod,
		SessionId:  e.SessionID,
		Ip:         e.IP,
		CreatedAt:  e.CreatedAt,
	}
}

func FromAuditLogList(logs []*core.AuditLogEntry, total, page, pageSize int) *AuditLogList {
	out := &AuditLogList{
		Logs:     make([]*AuditLogEntryDto, 0, len(logs)),
		Total:    int32(total),
		Page:     int32(page),
		PageSize: int32(pageSize),
	}
	for _, l := range logs {
		if converted := FromAuditLogEntry(l); converted != nil {
			out.Logs = append(out.Logs, converted)
		}
	}
	return out
}

func FromUserMessage(m *core.UserMessage) *UserMessageDto {
	if m == nil {
		return nil
	}
	payload := ""
	if len(m.Payload) > 0 {
		payload = string(m.Payload)
	}
	return &UserMessageDto{
		Id:           m.ID,
		Recipient:    m.Recipient,
		Sender:       m.Sender,
		Kind:         m.Kind,
		Severity:     m.Severity,
		Title:        m.Title,
		Body:         m.Body,
		Payload:      payload,
		ActionKind:   m.ActionKind,
		ActionStatus: m.ActionStatus,
		CreatedAt:    m.CreatedAt,
		ReadAt:       m.ReadAt,
		ActedAt:      m.ActedAt,
		ExpiresAt:    m.ExpiresAt,
	}
}

func FromUserMessageList(messages []*core.UserMessage, unreadCount int, nextCursor string) *UserMessageList {
	out := &UserMessageList{
		Messages:    make([]*UserMessageDto, 0, len(messages)),
		UnreadCount: int32(unreadCount),
		NextCursor:  nextCursor,
	}
	for _, m := range messages {
		if converted := FromUserMessage(m); converted != nil {
			out.Messages = append(out.Messages, converted)
		}
	}
	return out
}

func FromUnreadCount(count int) *UnreadCountResponse {
	return &UnreadCountResponse{
		UnreadCount: int32(count),
	}
}

func FromMarkAllRead(updated int64) *MarkAllReadResponse {
	return &MarkAllReadResponse{
		Ok:      true,
		Updated: updated,
	}
}

func FromClearMessages(deleted int64) *ClearMessagesResponse {
	return &ClearMessagesResponse{
		Ok:      true,
		Deleted: deleted,
	}
}

func FromSendNotification(sent int64) *SendNotificationResponse {
	return &SendNotificationResponse{
		Ok:   true,
		Sent: sent,
	}
}

func FromUserSearch(users []string) *UserSearchResponse {
	return &UserSearchResponse{
		Users: append([]string(nil), users...),
	}
}
