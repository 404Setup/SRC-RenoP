---
title: Status & Telemetry API
order: 9
category: API Reference
description: Health checks, system metrics, and diagnostic pprof endpoints
---

# Status & Telemetry API

## 1. Health Probe

- **Path**: `GET /api/status/health`
- **Auth**: None (Public)
- **Response**: `200 OK`, text body `"UP"`

---

## 2. System Status

- **Path**: `GET /api/status/system`
- **Auth**: Required (or Manager)

### Response (JSON)

```json
{
  "version": "1.0.0",
  "go_version": "go1.28-404setup",
  "uptime_seconds": 86400,
  "memory": {
    "alloc_bytes": 33554432,
    "total_alloc_bytes": 1073741824,
    "sys_bytes": 67108864,
    "num_gc": 120
  },
  "storage": {
    "total_artifacts": 1540,
    "storage_used_bytes": 5242880000
  }
}
```

---

## 3. Diagnostic Endpoints (`debug_mode: true`)

When `server.debug_mode: true` is set in `config.yaml`, pprof diagnostics are available under `/api/debug/`:

- `GET /api/debug/pprof/`: pprof index page
- `GET /api/debug/pprof/profile`: CPU profile sampling
- `GET /api/debug/pprof/heap`: Heap memory profile
- `GET /api/debug/pprof/goroutine`: Goroutine stack dump
