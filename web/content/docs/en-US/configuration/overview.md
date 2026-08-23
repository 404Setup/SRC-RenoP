---
title: Configuration Overview
order: 1
category: Configuration
description: Overview of config.yaml parameters, server settings, and environment variables
---

# Configuration Overview

The primary configuration file for RenoP is `config.yaml`. By default, it is read from the process working directory or
overridden by the `RENOP_CONFIG` environment variable.

## Configuration Files

| File                | Environment Variable | Description                                                                |
|:--------------------|:---------------------|:---------------------------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | Server ports, TLS, database connection, storage path, proxies, and updater |
| `repositories.yaml` | `RENOP_REPOSITORIES` | Repository definitions, visibility, upstream mirrors, and S3 configs       |
| `tokens.yaml`       | `RENOP_TOKENS`       | Bootstrap user accounts and static tokens (migrated to DB on startup)      |
| `index.json`        | `RENOP_INDEX`        | Search metadata index cache                                                |
| `sessions.bin`      | `RENOP_SESSIONS`     | Active web session cache                                                   |

## `config.yaml` Schema

### Storage & Documentation Preview

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
```

| Parameter                | Default   | Description                                                |
|:-------------------------|:----------|:-----------------------------------------------------------|
| `storage_path`           | `storage` | Root directory for local artifact storage                  |
| `enable_javadoc_preview` | `true`    | Enables HTML extraction and preview of Javadoc JARs        |
| `javadoc_extract_path`   | `""`      | Extraction cache directory (uses system cache when empty)  |
| `max_javadoc_size_mb`    | `48`      | Maximum allowable size (MB) for extracted Javadoc archives |

### `server` Network & Security

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains:
    - "localhost"
  cors_origins: []
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: []
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
  gpg:
    key_servers:
      - "https://keys.openpgp.org"
      - "https://keyserver.ubuntu.com"
```

| Parameter             | Default            | Description                                                   |
|:----------------------|:-------------------|:--------------------------------------------------------------|
| `host`                | `0.0.0.0`          | IP address to bind                                            |
| `port`                | `3000`             | Port to listen on                                             |
| `ssl_enabled`         | `false`            | Enables built-in TLS/HTTPS                                    |
| `ssl_cert_path`       | `""`               | Path to TLS certificate (`.crt` or `.pem`)                    |
| `ssl_key_path`        | `""`               | Path to TLS private key (`.key`)                              |
| `domains`             | `["localhost"]`    | Public hostnames used for download links and default CORS     |
| `cors_origins`        | `[]`               | Allowed CORS origins (empty = `domains` only, `*` = all)      |
| `enable_compression`  | `false`            | Enables gzip/brotli HTTP response compression                 |
| `file_cache_size_mb`  | `16`               | In-memory cache size for small static files and metadata      |
| `max_active_requests` | `512`              | Max concurrent active requests (returns 503 when exceeded)    |
| `trusted_proxies`     | `[]`               | Trusted reverse proxy IPs or CIDRs                            |
| `cdn_ip_header`       | `X-Forwarded-For`  | Request header containing real client IP from trusted proxies |
| `debug_mode`          | `false`            | Enables `/api/debug` pprof diagnostic endpoints               |
| `gpg.key_servers`     | Default keyservers | OpenPGP keyservers used for signature verification            |

> **Note**: Modifying `host`, `port`, or TLS settings requires restarting the RenoP process.

### `database` Connection

```yaml
database:
  enabled: true
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

Supports `sqlite3`, `mysql`, and `postgres`. See [Database Configuration](./database.md) for details.

### `proxy` Outbound Proxy

```yaml
proxy:
  selected: ""
  proxies:
    corp_proxy:
      url: "http://proxy.internal:8080"
```

Configures outbound HTTP/HTTPS/SOCKS5 proxies for upstream mirroring.
See [Outbound Proxy Configuration](./outbound-proxy.md).

### `frontend` Branding

```yaml
frontend:
  id: "renop"
  title: "RenoP Package Registry"
  description: "Self-hosted package repository"
  organization_website: ""
  organization_logo: "/svg/logo.svg"
  background_url: ""
  icp_license: ""
  public_security_filing: ""
  legal_notice_url: ""
```

Customizes page title, logo, organization URL, and footer compliance text.

### `updater` Auto-Updates

```yaml
updater:
  channel: "release"    # release or nightly
  mode: "manual"        # manual updates via UI button
```
