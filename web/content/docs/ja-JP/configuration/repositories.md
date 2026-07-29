---
title: Repositories & mirrors
order: 2
category: Configuration
description: repositories.yaml — visibility, mirrors, and S3
---

# Repositories & mirrors

File: `repositories.yaml` (override with `RENOP_REPOSITORIES`).

Default repositories:

| Name        | Role                    |
|-------------|-------------------------|
| `releases`  | Releases (usually PUBLIC)  |
| `snapshots` | Snapshots (usually PUBLIC) |
| `private`   | Private (PRIVATE)       |

Keyed by name under `repositories:`.

## Repository fields

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC          # PUBLIC | HIDDEN | PRIVATE
    allow_redeployment: false
    mirrors: [ ]
    s3:
      enabled: false
      endpoint: ""
      bucket: ""
      region: auto
      access_key_id: ""
      secret_access_key: ""
      force_path_style: true
      redirect_downloads: false
```

| Field                | Description                                                                           |
|----------------------|---------------------------------------------------------------------------------------|
| `name`               | Repository id (path segment: `http://host:port/{name}/…`)                             |
| `visibility`         | `PUBLIC` anonymous read; `HIDDEN` restricted listing; `PRIVATE` needs read permission |
| `allow_redeployment` | Whether overwriting an existing artifact path is allowed (defaults: releases/private `false`, snapshots `true`) |
| `mirrors`            | Upstream Maven proxies (optional)                                                     |
| `s3`                 | Optional S3-compatible backend for this repository                                    |

Maven layout under each repo is standard: `group/artifact/version/file`.

## Mirrors

On miss, mirrors fetch from upstream and may cache the result.

| Field             | Description                                                            |
|-------------------|------------------------------------------------------------------------|
| `name`            | Display / config name                                                  |
| `url`             | Upstream base URL                                                      |
| `persist`         | Persist cached artifacts to storage                                    |
| `cache_ttl_secs`  | Positive cache TTL (seconds)                                           |
| `negative_cache`  | Cache “not found” responses                                            |
| `timeout_secs`    | Upstream request timeout                                               |
| `authorization`   | Optional credentials (`method`, `login`, `password`)                   |
| `enabled_date`    | Optional activation date string                                        |
| `allow_artifacts` | If set, only matching `group` or `group:artifact` patterns are proxied |
| `deny_artifacts`  | If set, matching coordinates are blocked (do not combine with allow)   |

Authorization methods commonly used: `BASIC` / username-password, or `Bearer` / token.

## Visibility vs permissions

| Visibility | Anonymous read                                      | Notes                        |
|------------|-----------------------------------------------------|------------------------------|
| PUBLIC     | Yes                                                 | Open repo                    |
| HIDDEN     | File fetch may work; root listing needs extra roles |                              |
| PRIVATE    | No                                                  | Needs `canview` / `allview` / `proview`, write rights, or manager |

Writes always require `canupdate` (or manager). See [Authentication](../api/authentication.md).

## S3-compatible storage

When `s3.enabled` is true, artifacts for that repository are stored in the given bucket. Typical fields:

| Field                                 | Description                                    |
|---------------------------------------|------------------------------------------------|
| `endpoint`                            | S3 API endpoint                                |
| `bucket`                              | Bucket name                                    |
| `region`                              | Region (or `auto`)                             |
| `access_key_id` / `secret_access_key` | Credentials                                    |
| `force_path_style`                    | Path-style URLs (common for MinIO)             |
| `redirect_downloads`                  | Redirect clients to object URLs when supported |

## See also

- [Configuration overview](./overview.md)
- [Storage API](../api/storage.md)
- [Maven client](../getting-started/maven-client.md)
