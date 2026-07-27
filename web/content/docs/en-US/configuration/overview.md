---
title: Configuration overview
order: 1
category: Configuration
description: Config files, server settings, and environment variables
---

# Configuration overview

RenoP stores configuration and state next to the process working directory. Paths can be overridden with environment
variables.

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

Root directory for local artifact storage (default layout under this path). Default relative path is typically
`storage`.

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

The website [Download](/download) page uses the same release and nightly sources.

## Management UI

Signed-in **manager** / **admin** accounts can edit many settings under **Settings** and **Repositories**. File-level
changes still apply after reload/restart as documented for each domain.

## Storage backends

Artifacts can live on:

- **Local disk** under `storage_path` (default)
- **S3-compatible object storage** (global or per repository in `repositories.yaml`)

Checksum sidecars (MD5 / SHA-1 / SHA-256 / SHA-512) can be generated on upload.

See [Repositories & mirrors](./repositories.md) for visibility, mirrors, and S3 fields.

## Related

- [Quick start](../getting-started/quickstart.md)
- [Maven client setup](../getting-started/maven-client.md)
- [API index](../api/README.md)
