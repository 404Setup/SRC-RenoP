---
title: Docker / OCI Registry v2 API
order: 6
category: API Reference
description: OCI Distribution Spec v2 and Docker Registry v2 API endpoints
---

# Docker / OCI Registry v2 API

RenoP implements the OCI Distribution Spec v2 and Docker Registry v2 specifications.

Container images are explicit resources. Create an image with `POST /api/docker/repositories/:repo/images` or the
repository page before requesting push credentials. Registry blob and manifest endpoints never create an image as a
side effect. Creation can mark an image private; private images are omitted from unauthorized catalogs and require an
explicit L0-L4 image membership or administrator access for manifests and referenced blobs.
Image creation returns `409 Conflict` when the normalized name is already used locally or by an applicable enabled
upstream mirror. It returns `503 Service Unavailable` instead of claiming the name when an upstream check is
inconclusive.

Browser-management endpoints keep a human-readable plain-text error body and also return `X-Renop-Error-Code` for
stable programmatic handling. The RenoP frontend maps this code through its active locale instead of displaying raw
server text. OCI Distribution endpoints continue to use the specification-defined structured `errors` response.

Image pages provide a package-level Markdown README. An L3/L4 image member or administrator updates it with
`PUT /api/docker/repositories/{repo}/images?image={name}`. The JSON `description` value is limited to 512 KiB and is
rendered through the shared element and URL allowlist.

## Version Check

- **Path**: `GET /v2/` or `HEAD /v2/`
- **Response**:
    - `200 OK` with header `Docker-Distribution-API-Version: registry/2.0`
    - `401 Unauthorized` with `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"` when authentication
      is required.

---

## Bearer Token Auth

- **Path**: `GET /v2/token` or `GET /v2/auth`
- **Description**: Exchanges Basic Auth credentials for a temporary Docker Bearer token. An API token must include
  `repository:read` for pull, `repository:publish` for push, and `repository:delete` for published manifest/blob
  deletion; image visibility and L0-L4 membership are checked independently before each requested action is granted.

---

## Catalog & Tags

### List Repositories

- **Path**: `GET /v2/_catalog`
- **Response (JSON)**: `{"repositories": ["my-org/my-app"]}`

### List Tags

- **Path**: `GET /v2/:name/tags/list`
- **Response (JSON)**: `{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## Manifest Operations

- **Fetch Manifest**: `GET /v2/:name/manifests/:reference`
- **Upload Manifest**: `PUT /v2/:name/manifests/:reference` (pre-created image and L1 or higher required)
- **Delete Manifest**: `DELETE /v2/:name/manifests/:reference`

---

## Blob Layer Operations

- **Check Blob**: `HEAD /v2/:name/blobs/:digest`
- **Download Blob**: `GET /v2/:name/blobs/:digest`
- **Start Upload**: `POST /v2/:name/blobs/uploads/` (supports `?mount=<digest>&from=<other_repo>`)
- **Append Chunk**: `PATCH /v2/:name/blobs/uploads/:uuid`
- **Commit Upload**: `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
