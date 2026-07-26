---
title: Status
order: 5
category: API
---

# Status and health

Prefix: `/api/status`

No authentication required.

## `GET /api/status/health`

```json
"UP"
```

Liveness probe.

## `GET /api/status/hash`

Frontend asset content hash as a JSON string (cache busting).

## `GET /api/status/instance`

Response: `application/x-protobuf`, `InstanceStatus`.

| Field                                                  | Meaning                                        |
|--------------------------------------------------------|------------------------------------------------|
| `version`                                              | Binary version                                 |
| `development`                                          | Development build flag                         |
| `uptime`                                               | Milliseconds since start                       |
| `used_memory` / `total_memory`                         | Memory, roughly MiB                            |
| `renop_used_disk`                                      | RenoP storage usage                            |
| `disk_used` / `disk_total`                             | Disk                                           |
| `used_threads` / `available_threads` / `total_threads` | Thread / goroutine related                     |
| `architecture` / `os`                                  | GOARCH / GOOS                                  |
| `logical_cores` / `physical_cores`                     | CPU                                            |
| `failures_count`                                       | Runtime failure counter                        |
| `update_state`                                         | Updater state — see [updater.md](./updater.md) |

## `GET /api/status/snapshots`

Historical samples. Response: protobuf `StatusSnapshotList`.

| Field          | Meaning           |
|----------------|-------------------|
| `timestamp`    | Unix milliseconds |
| `used_memory`  | Memory            |
| `used_threads` | Thread count      |
| `open_files`   | Open file count   |

Empty list when there is no data (not 404).
