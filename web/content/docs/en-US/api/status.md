---
title: Status & Telemetry API
order: 9
category: API Reference
description: Public health, runtime metrics, snapshots, and protected diagnostics
---

# Status & Telemetry API

Status responses use protobuf where noted. Health and runtime status are public; memory diagnostics require a manager
and `server.debug_mode` enabled when the process starts.

## Health and frontend hash

- **Health**: `GET /api/status/health` returns `"UP"` while the process is serving.
- **Frontend hash**: `GET /api/status/hash` returns the embedded asset hash used for reload detection.

## Current instance status

- **Path**: `GET /api/status/instance`
- **Format**: protobuf `InstanceStatus`.
- **Contents**: version, uptime, RSS/VSS memory, disk use, goroutines, CPU topology, failure count, debug state, and the
  updater state.

### Decoded example

```json
{
  "version": "1.0.0",
  "uptime": 86400,
  "used_memory": 33554432,
  "vss_memory": 268435456,
  "renop_used_disk": 5242880000,
  "disk_used": 107374182400,
  "disk_total": 536870912000,
  "used_threads": 24,
  "logical_cores": 16,
  "failures_count": 0,
  "debug_mode": false
}
```

## Historical snapshots and diagnostics

- **Snapshots**: `GET /api/status/snapshots` returns protobuf `StatusSnapshotList`. Samples contain timestamp, memory,
  goroutine, open-file, and VSS values.
- **Heap profile**: `GET /api/debug/memory/heap` (`?gc=0` skips the default GC).
- **Allocation profile**: `GET /api/debug/memory/allocs`.
- **Goroutine profile**: `GET /api/debug/memory/goroutine`.
- **Runtime breakdown**: `GET /api/debug/memory/runtime` (`?gc=1` runs GC first).

```json
{
  "snapshots": [
    {
      "timestamp": 1787731200000,
      "used_memory": 33554432,
      "used_threads": 24,
      "open_files": 18,
      "vss_memory": 268435456
    }
  ]
}
```

Binary pprof responses can be opened with `go tool pprof` or Speedscope. Diagnostic routes return `403` when debug mode
was not active at process start, even if the caller is an administrator.
