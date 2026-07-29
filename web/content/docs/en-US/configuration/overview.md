---
title: Configuration overview
order: 1
category: Configuration
description: Config files, server settings, and environment variables
---

# Configuration overview

Config and state sit in the process working directory. Override paths with env vars.

## Files

| File                | Env override         | Purpose                                                    |
|---------------------|----------------------|------------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | Server bind, TLS, frontend branding, storage path, updater |
| `repositories.yaml` | `RENOP_REPOSITORIES` | Repositories, mirrors, per-repo S3                         |
| `tokens.yaml`       | `RENOP_TOKENS`       | Users, roles, upload tokens                                |
| `index.json`        | `RENOP_INDEX`        | Artifact index cache                                       |
| `sessions.json`     | `RENOP_SESSIONS`     | Browser login sessions                                     |

Also related at runtime:

| Variable                       | Default   | Purpose                                |
|--------------------------------|-----------|----------------------------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | generated | Password for the first `admin` account |

## `config.yaml` structure

### `storage_path`

Local artifact storage root. Default relative path is usually `storage`.

### `server`

| Key                   | Default           | Description                                                       |
|-----------------------|-------------------|-------------------------------------------------------------------|
| `host`                | `0.0.0.0`         | Listen address                                                    |
| `port`                | `3000`            | Listen port                                                       |
| `ssl_enabled`         | `false`           | Enable TLS                                                        |
| `ssl_cert_path`       | `""`              | Certificate path when TLS is on                                   |
| `ssl_key_path`        | `""`              | Private key path when TLS is on                                   |
| `domains`             | `[localhost]`     | Public hostnames for this instance (UI/metadata + default CORS)   |
| `cors_origins`        | `[]`              | Browser CORS allow list (empty = only `domains`; `*` = any)       |
| `enable_compression`  | `false`           | HTTP response compression                                         |
| `file_cache_size_mb`  | `100`             | In-memory file cache size (MB)                                    |
| `max_active_requests` | `2000`            | Concurrent request cap (overload → 503)                           |
| `trusted_proxies`     | `[]`              | Extra reverse-proxy CIDR/IPs (loopback always trusted)            |
| `cdn_ip_header`       | `X-Forwarded-For` | Client IP header behind a trusted proxy (e.g. `CF-Connecting-IP`) |

#### CORS (`server.cors_origins`)

Controls which browser `Origin` values may call this server cross-origin (session cookies use
`Access-Control-Allow-Credentials`).

| Value | Effect |
|-------|--------|
| *(empty)* | Only origins whose host matches one of `server.domains` (any scheme/port) |
| `*.pkg.one` | Apex `pkg.one` and any subdomain (e.g. `mvnc.pkg.one`, `cdn.pkg.one`) |
| `https://app.example.com` | Exact full origin (scheme + host + port) |
| `partner.example.com` | That hostname with any scheme/port |
| `*` | Allow every origin |

Legacy configs with singular `domain: example.com` still load and migrate to `domains: [example.com]`.

Restart the process after changing host, port, or TLS settings.

### `frontend`

Branding for the embedded repository browser:

| Key                    | Description                      |
|------------------------|----------------------------------|
| `id`                   | Frontend / site id               |
| `title`                | Page title                       |
| `description`          | Short description                |
| `organization_website` | Org / product URL                |
| `organization_logo`    | Logo path (e.g. `/svg/logo.svg`) |
| `background_url`       | Optional background image        |
| `icp_license`          | Optional ICP / compliance text   |

### `updater`

| Key       | Default   | Description                                       |
|-----------|-----------|---------------------------------------------------|
| `channel` | `release` | `release` or `nightly`                            |
| `mode`    | `manual`  | How updates are applied (e.g. manual from the UI) |

Website [Download](/download) uses the same stable/nightly sources.

## Management UI

**manager** / **admin** can edit most things under Settings and Repositories. Some file edits need reload/restart (see each domain).

## Storage

- **Local disk** under `storage_path` (default)
- **S3-compatible** object storage (per repo in `repositories.yaml`)

Upload can write MD5 / SHA-1 / SHA-256 / SHA-512 sidecars.

Visibility, mirrors, S3 fields: [Repositories & mirrors](./repositories.md).

## See also

- [Quick start](../getting-started/quickstart.md)
- [Maven client](../getting-started/maven-client.md)
- [API index](../api/README.md)
