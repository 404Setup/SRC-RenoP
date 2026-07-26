---
title: Updater
order: 7
category: API
---

# Updater

Prefix: `/api/updater`

`GET /status` is public; `check` / `install` / `upload` / `restart` need **manager**.

The same state is also on `GET /api/status/instance` as `update_state`.

Typical flow:

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

Response: `application/x-protobuf`, `UpdateState` (see `proto/api/v1/api.proto`).

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

| Query     | Default   | Meaning                |
|-----------|-----------|------------------------|
| `channel` | `release` | `release` or `nightly` |

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

Check failure → 500, `{ "error": "…" }`.

## `POST /api/updater/install`

Async download and extract using the current `download_url`. If empty, falls back to the nightly default URL.

| Status | Reason                                                       |
|--------|--------------------------------------------------------------|
| 507    | Insufficient disk                                            |
| 409    | Install already running (`Installation already in progress`) |

Immediate success response:

```json
{"status": "started"}
```

Poll `/status` for progress. Finished state: `ready_to_restart`.

## `POST /api/updater/upload`

Offline update: multipart zip. Form field `file` or `package`; must be `.zip`.

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

This single-request multipart path remains the default for small packages and non-UI clients.

### Multi-part offline upload — optional

Large zip packages from the Dashboard offline-update dialog may use concurrent chunked upload via the shared session API
(manager only). Packages under **8 MiB** still use the single-request
`POST /api/updater/upload` path. Init/complete are **`application/x-protobuf`**
(`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`); parts are raw octets.

Part size is chosen dynamically from total size (see [storage.md](./storage.md) multi-part section); use the
`chunk_size` / `chunk_count` from the init response.

1. `POST /api/upload/chunked/` with `purpose=updater`, `filename` (must end with `.zip`), `size`
2. Parallel `PUT /api/upload/chunked/:id/:index` for each part (retry-safe; re-PUT of accepted parts is OK)
3. `POST /api/upload/chunked/:id/complete` — extracts the binary and sets `ready_to_restart`

Complete protobuf fields: `status=ready_to_restart`, `message=…`.

## `POST /api/updater/restart`

Replace the binary with the prepared update and restart.

Not ready → 400 (`No update ready to install`).

```json
{"status": "restarting"}
```

The connection drops afterward; that is expected.
