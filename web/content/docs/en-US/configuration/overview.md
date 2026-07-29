---
title: Configuration overview
order: 1
category: Configuration
description: Config files, server settings, and environment variables
---

# Configuration overview

Configuration files and runtime state are stored in the process working directory. Paths can be overridden with
environment variables.

## Files

| File                | Environment variable | Purpose                                                             |
|---------------------|----------------------|---------------------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | Listen address, TLS, frontend branding, storage path, updater       |
| `repositories.yaml` | `RENOP_REPOSITORIES` | Repositories, mirrors, per-repository S3                            |
| `tokens.yaml`       | `RENOP_TOKENS`       | Users, roles, upload tokens                                         |
| `index.json`        | `RENOP_INDEX`        | Artifact index cache                                                |
| `sessions.bin`      | `RENOP_SESSIONS`     | Browser login sessions (legacy `sessions.json` is migrated on load) |

Runtime-related:

| Variable                       | Default   | Purpose                                |
|--------------------------------|-----------|----------------------------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | generated | Password for the first `admin` account |

## `config.yaml` structure

### `storage_path`

Root directory for local artifact storage. The default relative path is `storage`.

### `server`

| Key                   | Default           | Description                                                                       |
|-----------------------|-------------------|-----------------------------------------------------------------------------------|
| `host`                | `0.0.0.0`         | Listen address                                                                    |
| `port`                | `3000`            | Listen port                                                                       |
| `ssl_enabled`         | `false`           | Enable TLS                                                                        |
| `ssl_cert_path`       | `""`              | Certificate path when TLS is enabled                                              |
| `ssl_key_path`        | `""`              | Private key path when TLS is enabled                                              |
| `domains`             | `[localhost]`     | Public hostnames for this instance (UI/metadata and default CORS)                 |
| `cors_origins`        | `[]`              | Browser CORS allow list (empty = `domains` only; `*` = any origin)                |
| `enable_compression`  | `false`           | Enable HTTP response compression                                                  |
| `file_cache_size_mb`  | `16`              | In-memory file cache size (MB)                                                    |
| `max_active_requests` | `512`             | Maximum concurrent requests (overload returns 503)                                |
| `trusted_proxies`     | `[]`              | Additional reverse-proxy CIDR/IP ranges (loopback is always trusted)              |
| `cdn_ip_header`       | `X-Forwarded-For` | Header used for client IP behind a trusted proxy (for example `CF-Connecting-IP`) |

#### CORS (`server.cors_origins`)

Controls which browser `Origin` values may call this server cross-origin. Session cookies are returned with
`Access-Control-Allow-Credentials`.

| Value                     | Effect                                                                       |
|---------------------------|------------------------------------------------------------------------------|
| *(empty)*                 | Only origins whose host matches one of `server.domains` (any scheme or port) |
| `*.pkg.one`               | Apex domain `pkg.one` and any subdomain (for example `mvnc.pkg.one`)         |
| `https://app.example.com` | Exact full origin (scheme, host, and port)                                   |
| `partner.example.com`     | That hostname with any scheme or port                                        |
| `*`                       | Allow every origin                                                           |

Legacy configurations that use the singular form `domain: example.com` still load and are migrated to
`domains: [example.com]`.

Restart the process after changing `host`, `port`, or TLS settings.

### `frontend`

Branding fields for the embedded repository browser:

| Key                    | Description                             |
|------------------------|-----------------------------------------|
| `id`                   | Frontend / site identifier              |
| `title`                | Page title                              |
| `description`          | Short description                       |
| `organization_website` | Organization or product URL             |
| `organization_logo`    | Logo path (for example `/svg/logo.svg`) |
| `background_url`       | Optional background image URL           |
| `icp_license`          | Optional ICP or compliance text         |

### `updater`

| Key       | Default   | Description                                                    |
|-----------|-----------|----------------------------------------------------------------|
| `channel` | `release` | `release` or `nightly`                                         |
| `mode`    | `manual`  | How updates are applied (for example manual install in the UI) |

The website [Download](/download) page and in-instance updates use the same class of stable and nightly release sources.

## Management UI

Accounts with **manager** or **admin** permissions can change most settings under Settings and Repositories. Some
configuration changes require a reload or process restart after the file is written. See the documentation for each
configuration domain.

## Storage

- **Local disk** under `storage_path` (default)
- **S3-compatible** object storage, configured per repository in `repositories.yaml`

Uploads can write MD5 / SHA-1 / SHA-256 / SHA-512 sidecar checksum files.

For visibility, mirrors, and S3 fields, see [Repositories & mirrors](./repositories.md).

## See also

- [Quick start](../getting-started/quickstart.md)
- [Maven client](../getting-started/maven-client.md)
- [API index](../api/README.md)
