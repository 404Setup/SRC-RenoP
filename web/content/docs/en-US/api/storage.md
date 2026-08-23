---
title: Storage & Upload API
order: 10
category: API Reference
description: Standard Maven HTTP operations and chunked artifact uploads
---

# Storage & Upload API

## 1. Standard Maven HTTP Operations

Clients interact directly with repository paths: `/{repo}/{path...}`

### Download Artifact

- **Request**: `GET /{repo}/{path}`
- **Description**: Supports HTTP Range requests, generating ETag and Last-Modified headers.

### Upload Artifact

- **Request**: `PUT /{repo}/{path}`
- **Auth**: Requires write permission (`canupdate:{repo}`)
- **Response**: `201 Created`

### Delete Artifact

- **Request**: `DELETE /{repo}/{path}`
- **Auth**: Requires admin permission (`canadmin:{repo}`)
- **Response**: `204 No Content`

---

## 2. Chunked Large File Uploads

For large packages and archives, clients can perform chunked uploads.

### Initialize Upload

- **Path**: `POST /api/upload/chunked`
- **Request Body (JSON)**:
  ```json
  {
    "repository": "releases",
    "target_path": "com/example/big-app/1.0.0/big-app-1.0.0.jar",
    "total_size": 524288000,
    "chunk_size": 10485760
  }
  ```
- **Response (JSON)**: `{"upload_id": "up_987654321", "chunk_size": 10485760}`

### Upload Chunk

- **Path**: `PUT /api/upload/chunked/:upload_id?chunk_index=0`
- **Body**: Binary byte stream for the chunk

### Finalize & Verify

- **Path**: `POST /api/upload/chunked/:upload_id/complete`
- **Request Body (JSON)**:
  ```json
  {
    "sha256": "abcdef1234567890..."
  }
  ```
- **Response**: `201 Created`
