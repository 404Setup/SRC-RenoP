---
title: In-Place Updater API
order: 13
category: API Reference
description: Checking updates, channel switching, and applying updates
---

# In-Place Updater API

## 1. Check Update Status

- **Path**: `GET /api/updater/status`
- **Auth**: Admin

### Response (JSON)

```json
{
  "current_version": "1.0.0",
  "channel": "release",
  "has_update": true,
  "latest_version": "1.1.0",
  "release_notes": "Bug fixes and Docker chunked upload optimizations.",
  "release_date": "2026-08-20T10:00:00Z"
}
```

---

## 2. Apply Update

- **Path**: `POST /api/updater/apply`
- **Auth**: Admin
- **Description**: Downloads the binary for the current platform and CPU microarchitecture, verifies SHA256 checksums,
  and replaces the running executable.
- **Response**: `200 OK`, `{"success": true, "message": "Update applied successfully. Please restart the service."}`
