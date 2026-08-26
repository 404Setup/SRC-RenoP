---
title: Storage & Upload API
order: 10
category: API Reference
description: Direct repository operations and bounded resumable uploads
---

# Storage & Upload API

Direct storage routes apply to Maven and `files` repositories. Cargo and Docker use their native protocol APIs. Every
mutation is checked against API-token scope, repository permission, repository format, and Maven-domain policy.

## Direct repository operations

The canonical path is `/{repo}/{path...}`. Reads support HTTP validators and byte ranges. `HIDDEN` repositories are
unlisted but exact paths remain readable; `PRIVATE` repositories require authorization.

### Download

- **Request**: `GET /{repo}/{path}` or `HEAD /{repo}/{path}`
- Missing local files may be resolved through an enabled mirror and streamed into the configured cache policy.

### Upload

- **Request**: `PUT /{repo}/{path}`
- **Auth**: Password or API token with `repository:publish`, plus current write/domain permission.
- Maven accepts only valid coordinates and metadata under a verified domain. `files` accepts sanitized arbitrary paths
  and supports replacement.

### Delete

- **Request**: `DELETE /{repo}/{path}`
- **Auth**: API token with `repository:delete` or another allowed credential, plus current delete permission.

## Chunked resumable uploads

Chunked uploads use protobuf metadata and raw binary parts. The server owns the final destination, bounds part size and
session count, and deletes abandoned temporary files.

### Initialize

- **Path**: `POST /api/upload/chunked/`
- **Content-Type**: `application/x-protobuf` with `ChunkedUploadInitRequest`.
- `purpose` is `storage` or `updater`. Storage `path` includes the repository name.

```json
{
  "purpose": "storage",
  "filename": "app-1.0.0.jar",
  "size": 524288000,
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "generate_checksums": true,
  "chunk_size": 4194304,
  "gpg_signature_expected": false
}
```

### Upload a part

- **Path**: `PUT /api/upload/chunked/{upload_id}/{index}`
- **Content-Type**: `application/octet-stream`.
- Parts may run concurrently. Retrying an already accepted index is idempotent; a part with the wrong length is rejected.

### Complete or abort

- **Complete**: `POST /api/upload/chunked/{upload_id}/complete`
- **Abort**: `DELETE /api/upload/chunked/{upload_id}`
- Completion is single-winner. It verifies every part, rechecks authorization, and commits through the repository gate.

```json
{
  "status": "created",
  "message": "",
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "release_id": ""
}
```

When Maven requires GPG, completion may return `202 Accepted` with a `release_id` while publication remains quarantined.
For `purpose=updater`, success returns `ready_to_restart` instead of a repository path.
