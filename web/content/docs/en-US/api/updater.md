---
title: Updater
order: 7
category: API
---

# Updater

Prefix: `/api/updater`

`GET /status` is public; `check` / `install` / `upload` / `restart` need **manager**.

Same state on `GET /api/status/instance` as `update_state`.

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

Response: `application/x-protobuf`, `UpdateState` (`proto/api/v1/api.proto`).

| Field                  | Meaning                                                         |
|------------------------|-----------------------------------------------------------------|
| `status`               | `idle`, `available`, `downloading`, `ready_to_restart`, `error` |
| `latest_version`       | Latest version string                                           |
| `download_url`         | Package download URL                                            |
| `progress`             | 0–100 while downloading                                         |
| `error_message`        | Set when `status` is `error`                                    |
| `size`                 | Package size (bytes)                                            |
| `estimated_disk_space` | Estimated free space needed (bytes)                             |
| `release_date`         | Release timestamp string                                        |
| `release_notes`        | Release notes text                                              |
| `commit_sha`           | Source commit                                                   |
| `is_release`           | Release channel build                                           |

## `POST /api/updater/check`

| Query     | Default                   | Meaning                |
|-----------|---------------------------|------------------------|
| `channel` | `updater.channel` setting | `release` or `nightly` |

Omit / invalid → `updater.channel` (default `release`).

| Channel   | `info.json`                                           |
|-----------|-------------------------------------------------------|
| `nightly` | `https://mvnc.pkg.one/update/renop/nightly/info.json` |
| `release` | `https://mvnc.pkg.one/update/renop/stable/info.json`  |

Packages: `…/{nightly\|stable}/{version}/{file}`.

```json
{
  "has_update": true,
  "current_version": "…",
  "latest_version": "…",
  "download_url": "…",
  "channel": "release",
  "size": 12345678,
  "estimated_disk_space": 40000000,
  "release_date": "…",
  "release_notes": "…",
  "commit_sha": "…",
  "is_release": true
}
```

Failure → 500, `{ "error": "…" }`.

## `POST /api/updater/install`

Async download/extract using current `download_url`.

| Status | Reason                                                       |
|--------|--------------------------------------------------------------|
| 507    | Insufficient disk                                            |
| 409    | Install already running (`Installation already in progress`) |

```json
{"status": "started"}
```

Poll `/status`. Done: `ready_to_restart`.

## `POST /api/updater/upload`

Offline update: multipart zip (`file` or `package`). Must be `.zip`.

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

### Multi-part upload (optional)

Large zips may use chunked upload (manager). Under **8 MiB** → single `POST /api/updater/upload`.

Init/complete: **`application/x-protobuf`** (`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`). Parts: raw
bytes.

Part size: see [storage.md](./storage.md). Use `chunk_size` / `chunk_count` from init.

1. `POST /api/upload/chunked/` — `purpose=updater`, `filename` (`.zip`), `size`
2. `PUT /api/upload/chunked/:id/:index` (parallel, retry-safe)
3. `POST /api/upload/chunked/:id/complete` → `ready_to_restart`

## `POST /api/updater/restart`

If a prepared update binary is pending, applies it and restarts the process. Otherwise restarts the current process
without applying an update.

```json
{"status": "restarting"}
```
