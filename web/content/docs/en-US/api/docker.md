---
title: Docker / OCI Registry v2 API
order: 6
category: API Reference
description: OCI Distribution Spec v2 and Docker Registry v2 API endpoints
---

# Docker / OCI Registry v2 API

RenoP implements the OCI Distribution Spec v2 and Docker Registry v2 specifications.

## 1. Version Check

- **Path**: `GET /v2/` or `HEAD /v2/`
- **Response**:
    - `200 OK` with header `Docker-Distribution-API-Version: registry/2.0`
    - `401 Unauthorized` with `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"` when authentication
      is required.

---

## 2. Bearer Token Auth

- **Path**: `GET /v2/token` or `GET /v2/auth`
- **Description**: Exchanges Basic Auth credentials for a temporary Docker Bearer token.

---

## 3. Catalog & Tags

### List Repositories

- **Path**: `GET /v2/_catalog`
- **Response (JSON)**: `{"repositories": ["my-org/my-app"]}`

### List Tags

- **Path**: `GET /v2/:name/tags/list`
- **Response (JSON)**: `{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## 4. Manifest Operations

- **Fetch Manifest**: `GET /v2/:name/manifests/:reference`
- **Upload Manifest**: `PUT /v2/:name/manifests/:reference`
- **Delete Manifest**: `DELETE /v2/:name/manifests/:reference`

---

## 5. Blob Layer Operations

- **Check Blob**: `HEAD /v2/:name/blobs/:digest`
- **Download Blob**: `GET /v2/:name/blobs/:digest`
- **Start Upload**: `POST /v2/:name/blobs/uploads/` (supports `?mount=<digest>&from=<other_repo>`)
- **Append Chunk**: `PATCH /v2/:name/blobs/uploads/:uuid`
- **Commit Upload**: `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
