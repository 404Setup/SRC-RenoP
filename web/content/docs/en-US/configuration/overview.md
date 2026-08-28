---
title: Configuration Overview
order: 1
category: Configuration
description: Configuration files, server settings, storage, proxies, branding, and updates
---

# Configuration Overview

RenoP reads `config.yaml` from the working directory unless `RENOP_CONFIG` overrides it. Settings written through the
administrator UI use the same validated structures and private-file permissions.

## Configuration files

| File | Override | Purpose |
|:-----|:---------|:--------|
| `config.yaml` | `RENOP_CONFIG` | Server, database, previews, proxy, frontend, audit, and updater |
| `repositories.yaml` | `RENOP_REPOSITORIES` | Repository engines, visibility, mirrors, Maven policy, and S3 |
| `index.json` | `RENOP_INDEX` | Persisted file-index snapshot rebuilt from storage when required |

Accounts, API tokens, sessions, teams, audit logs, and messages are database records. They are not configured in YAML.
Keep configuration and repository files readable only by the service account because they may contain credentials.

## `config.yaml` schema

### Storage and documentation previews

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
enable_cargodoc_preview: true
cargodoc_extract_path: ""
max_cargodoc_size_mb: 128
```

An empty extraction path uses the platform cache directory. Extraction validates paths and size limits before content
is exposed through `/javadoc` or `/cargodoc`.

### `server` network and security

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains: ["localhost"]
  cors_origins: []
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: []
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
  gpg:
    key_servers: ["https://keys.openpgp.org", "https://keyserver.ubuntu.com"]
```

`domains` supplies public hostnames and default CORS hosts. `cors_origins` may add exact origins, hosts, or wildcard
hosts; `*` allows every origin. Forwarded client-IP headers are trusted only when the immediate peer matches
`trusted_proxies`. Host, port, TLS, compression, debug mode, and some cache changes require a restart.

GitHub OAuth is also stored under `server.github_oauth`; configure its client ID and write-only secret from the UI.

### `database` connection

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

Supported drivers are `sqlite3` (or `sqlite`), `mysql`, `postgres`, and native `clickhouse`. See
[Database Configuration](./database.md).

### `proxy` outbound routing

```yaml
proxy:
  selected: ""
  proxies:
    - name: "corp_proxy"
      url: "http://proxy.internal:8080"
      username: ""
      password: ""
```

The list supports up to 16 HTTP, HTTPS, or SOCKS5 proxies. See
[Outbound Proxy Configuration](./outbound-proxy.md).

### `frontend` branding

```yaml
frontend:
  id: "renop"
  title: "RenoP Package Registry"
  description: "Self-hosted package repository"
  organization_website: ""
  organization_logo: "/svg/logo.svg"
  background_url: ""
  font_preset: "system"
  font_url: ""
  icp_license: ""
  public_security_filing: ""
  legal_notice_url: ""
```

Branding URLs are validated before use. Background images must satisfy the configured WebP and size policy.
`font_preset` accepts `system`, `inter`, `noto_sans`, `open_sans`, `source_sans`, or `custom`. Presets use locally
installed fonts. Custom font files are fetched asynchronously from a same-origin path or an HTTP(S) URL and activate
only after the complete font has loaded, so they do not block initial rendering.

### `updater` policy

```yaml
updater:
  channel: "release"
  mode: "manual"
```

`channel` is `release` or `nightly`. `mode` is `manual`, `auto_check`, or `auto_install`. Automatic checks are coalesced
by the process-wide scheduler; update results are delivered to administrators through the message center.
