---
title: Settings API
order: 8
category: API Reference
description: Server configuration, repository management, and index rebuild endpoints
---

# Settings API

## 1. Get Configuration

- **Path**: `GET /api/settings/config`
- **Auth**: Manager or Admin

---

## 2. Update Configuration

- **Path**: `PUT /api/settings/config`
- **Auth**: Admin
- **Description**: Updates server parameters, domain names, proxies, and branding. Host, port, and TLS changes require
  restarting the process.

---

## 3. Repository Settings

### Get All Repositories

- **Path**: `GET /api/settings/maven/repositories`
- **Auth**: Manager or Admin

### Update Repository

- **Path**: `PUT /api/settings/maven/repositories/:name`
- **Auth**: Manager or Admin

---

## 4. Rebuild Search Index

- **Path**: `POST /api/settings/index/rebuild`
- **Auth**: Admin
- **Description**: Asynchronously scans storage and rebuilds the `index.json` search cache.
- **Response**: `202 Accepted`, `{"message": "Index rebuild triggered"}`
