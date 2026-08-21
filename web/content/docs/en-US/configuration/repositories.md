---
title: Repositories & mirrors
order: 2
category: Configuration
description: repositories.yaml — visibility, mirrors, and S3
---

# Repositories & mirrors

File: `repositories.yaml` (override with `RENOP_REPOSITORIES`).

Default repositories:

| Name        | Role                       |
|-------------|----------------------------|
| `releases`  | Releases (usually PUBLIC)  |
| `snapshots` | Snapshots (usually PUBLIC) |
| `private`   | Private (PRIVATE)          |

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
      key_prefix: ""
      region: auto
      access_key_id: ""
      secret_access_key: ""
      force_path_style: true
      redirect_downloads: false
```

| Field                   | Description                                                                                                     |
|-------------------------|-----------------------------------------------------------------------------------------------------------------|
| `name`                  | Repository id (path segment: `http://host:port/{name}/…`)                                                       |
| `visibility`            | `PUBLIC` anonymous read; `HIDDEN` restricted listing; `PRIVATE` needs read permission                           |
| `allow_redeployment`    | Whether overwriting an existing artifact path is allowed (defaults: releases/private `false`, snapshots `true`) |
| `require_gpg_signature` | Require detached GPG signatures for `.jar`, `.pom`, and `.module` uploads; publication waits for verification   |
| `mirrors`               | Upstream Maven proxies (optional)                                                                               |
| `s3`                    | Optional S3-compatible backend for this repository                                                              |

Maven layout under each repo is standard: `group/artifact/version/file`.

## Mirrors

On miss, mirrors fetch from upstream and may cache the result.

| Field             | Description                                                                              |
|-------------------|------------------------------------------------------------------------------------------|
| `name`            | Display / config name                                                                    |
| `url`             | Upstream base URL                                                                        |
| `persist`         | Persist cached artifacts to storage                                                      |
| `cache_ttl_secs`  | Positive cache TTL (seconds)                                                             |
| `negative_cache`  | Cache “not found” responses                                                              |
| `timeout_secs`    | Upstream request timeout                                                                 |
| `authorization`   | Optional credentials (`method`, `login`, `password`)                                     |
| `proxy`           | Empty inherits the global proxy; `direct` bypasses it; a name selects a configured proxy |
| `enabled_date`    | Optional activation date string                                                          |
| `allow_artifacts` | If set, only matching `group` or `group:artifact` patterns are proxied                   |
| `deny_artifacts`  | If set, matching coordinates are blocked (do not combine with allow)                     |

Authorization methods commonly used: `BASIC` / username-password, or `Bearer` / token.

Mirror proxy credentials are no longer stored in `repositories.yaml`. Configure named proxies in the global `proxy`
settings domain and use the single mirror `proxy` selector described above.

## Visibility vs permissions

| Visibility | Anonymous read                                      | Notes                                                             |
|------------|-----------------------------------------------------|-------------------------------------------------------------------|
| PUBLIC     | Yes                                                 | Open repo                                                         |
| HIDDEN     | File fetch may work; root listing needs extra roles |                                                                   |
| PRIVATE    | No                                                  | Needs `canview` / `allview` / `proview`, write rights, or manager |

Writes always require `canupdate` (or manager). See [Authentication](../api/authentication.md).

## S3-compatible storage

When `s3.enabled` is true, artifacts for that repository are stored in the given bucket. Typical fields:

| Field                                 | Description                                    |
|---------------------------------------|------------------------------------------------|
| `endpoint`                            | S3 API endpoint                                |
| `bucket`                              | Bucket name                                    |
| `key_prefix`                          | Optional object key prefix within the bucket   |
| `region`                              | Region (or `auto`)                             |
| `access_key_id` / `secret_access_key` | Credentials                                    |
| `force_path_style`                    | Path-style URLs (common for MinIO)             |
| `redirect_downloads`                  | Redirect clients to object URLs when supported |

When `key_prefix` is empty, RenoP preserves the legacy object layout. Before adding or changing a prefix on a repository
that already contains artifacts, move its existing objects to the new prefix; RenoP does not migrate them automatically.

## See also

- [Configuration overview](./overview.md)
- [Storage API](../api/storage.md)
- [GPG signatures](../api/gpg.md)
- [Maven client](../getting-started/maven-client.md)
