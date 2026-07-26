---
title: Settings
order: 6
category: API
---

# Settings and repository config

Prefix: `/api/settings`

Read and write require **manager / admin**.

All request/response bodies under this prefix that carry structured data use **`application/x-protobuf`** (see
`proto/api/v1/api.proto`). Success bodies that are empty remain plain text (`""`). Validation errors remain short
English text.

On-disk locations:

| Content            | File                | Env var              |
|--------------------|---------------------|----------------------|
| Domain settings    | `config.yaml`       | `RENOP_CONFIG`       |
| Maven repositories | `repositories.yaml` | `RENOP_REPOSITORIES` |

Listener / TLS changes need a process restart to fully apply.

## Index

### `POST /api/settings/index/rebuild`

Request: protobuf `RebuildIndexRequest`

| Field  | Type   | Values           |
|--------|--------|------------------|
| `mode` | string | `full` \| `diff` |

| mode   | Behavior                                  |
|--------|-------------------------------------------|
| `full` | Async full rebuild; clears Javadoc caches |
| `diff` | Differential rebuild                      |

Anything else → 400 (`Invalid mode. Expected 'full' or 'diff'`). Success: 200, empty string body.

## Config domains

### `GET /api/settings/domains`

Response: protobuf `SettingsDomainsResponse`

| Field     | Type            |
|-----------|-----------------|
| `domains` | repeated string |

Typical values: `frontend`, `server`, `storage`, `updater`, `index`.

`index` currently has no configurable fields.

### `GET /api/settings/domain/:name`

Response: protobuf message for that domain (Content-Type `application/x-protobuf`).

**frontend** → `FrontendConfig`

| Field                  | Type   |
|------------------------|--------|
| `id`                   | string |
| `title`                | string |
| `description`          | string |
| `organization_website` | string |
| `organization_logo`    | string |
| `background_url`       | string |
| `icp_license`          | string |

**server** → `ServerConfig`

| Field                 | Type            |
|-----------------------|-----------------|
| `host`                | string          |
| `port`                | uint32          |
| `ssl_enabled`         | bool            |
| `ssl_cert_path`       | string          |
| `ssl_key_path`        | string          |
| `domain`              | string          |
| `enable_compression`  | bool            |
| `file_cache_size_mb`  | uint32          |
| `max_active_requests` | uint32          |
| `trusted_proxies`     | repeated string |
| `cdn_ip_header`       | string          |

**storage** → `StorageConfig`

| Field                    | Type   |
|--------------------------|--------|
| `storage_path`           | string |
| `enable_javadoc_preview` | bool   |
| `javadoc_extract_path`   | string |
| `max_javadoc_size_mb`    | int64  |

**updater** → `UpdaterConfig`

| Field     | Type   | Values                                                       |
|-----------|--------|--------------------------------------------------------------|
| `channel` | string | `release` \| `nightly`                                       |
| `mode`    | string | `manual` \| `auto_check` \| `auto_install` \| `safe_install` |

**index** → empty `IndexDomainSettings`

### `PUT /api/settings/domain/:name`

**Full replace** of the domain. Body is the same protobuf message as GET for that domain. Proto3 omitted fields decode
as zero values — clients must send the complete domain configuration (the UI always POSTs the full form state).

Success: 200, empty string.

Rules:

- `frontend.background_url`: when non-empty, must be reachable, public IP, WebP, ≤ 5 MiB; private addresses rejected
- `storage.max_javadoc_size_mb`: must be > 0
- `storage.storage_path`: when changed to a different path, the server immediately fully rebuilds the file index for the
  new root (and restarts the FS watcher); Javadoc caches are cleared
- `updater.channel` / `updater.mode`: must be one of the allowed enum values (empty is invalid)
- `index`: nothing writable → 404

Validation failure → 400 + short English error text.

## Maven repositories

### `GET /api/settings/maven/repositories`

Response: protobuf `MavenRepositoriesResponse` (`map<string, Repository>`).

| Field                | Meaning                                                   |
|----------------------|-----------------------------------------------------------|
| `name`               | Repository name                                           |
| `visibility`         | `PUBLIC` / `HIDDEN` / `PRIVATE`                           |
| `allow_redeployment` | Whether existing artifacts may be overwritten             |
| `mirrors[]`          | Upstream mirrors (url, persist, TTL, auth, allow/deny, …) |
| `s3`                 | Optional S3-compatible storage                            |

### `PUT /api/settings/maven/repositories/:name`

Create or **full replace**. Body is protobuf `Repository`. Path `:name` wins over body `name`.

Reserved names: `css`, `js`, `svg`, `api`, `javadocs`, `assets`, plus invalid characters.

Success: 200, empty string.

### `DELETE /api/settings/maven/repositories/:name`

Remove from config; **does not** delete files on disk. Success: 200, empty string.
