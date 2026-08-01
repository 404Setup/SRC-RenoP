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
| `used_memory` / `total_memory`                         | Physical memory used and total (bytes)         |
| `vss_memory`                                           | Virtual memory size (bytes)                    |
| `renop_used_disk`                                      | RenoP storage usage                            |
| `disk_used` / `disk_total`                             | Disk used and total                            |
| `used_threads` / `available_threads` / `total_threads` | Goroutines and request concurrency threads     |
| `architecture` / `os`                                  | GOARCH / GOOS                                  |
| `logical_cores` / `physical_cores`                     | Logical and physical CPU core counts           |
| `failures_count`                                       | Runtime failure counter                        |
| `update_state`                                         | Updater state — see [updater.md](./updater.md) |
| `debug_mode`                                           | Whether Debug mode was active at process start |

## `GET /api/status/snapshots`

Historical samples. Response: protobuf `StatusSnapshotList`.

| Field          | Meaning           |
|----------------|-------------------|
| `timestamp`    | Unix milliseconds |
| `used_memory`  | Memory            |
| `used_threads` | Thread count      |
| `open_files`   | Open file count   |

Empty list when there is no data (not 404).

## Debug analysis APIs (`/api/debug`)

Requires **manager** permission and `server.debug_mode: true` in the configuration file at startup. Returns 403 if debug
mode is disabled or permissions are insufficient.

### `GET /api/debug/memory/heap`

Dumps Go runtime heap profile (pprof format). Accepts query parameter `gc=1` (default: run GC before sampling).

### `GET /api/debug/memory/allocs`

Dumps memory allocations profile (pprof format).

### `GET /api/debug/memory/goroutine`

Dumps Goroutine stack profile (pprof format).

### `GET /api/debug/memory/runtime`

Returns Go runtime memory, heap/stack breakdown, and off-heap estimates. Response: `application/x-protobuf`,
`RuntimeMemoryBreakdown`. Includes fields such as `process_rss`, `process_vss`, `go_retained`, `heap_inuse`,
`heap_alloc`, `heap_sys`, and `off_heap_runtime_estimate`.
