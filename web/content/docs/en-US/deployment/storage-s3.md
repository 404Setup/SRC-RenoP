---
title: Storage Architecture
order: 3
category: Deployment
description: Local filesystem storage and S3-compatible object storage backends
---

# Storage Architecture

RenoP supports both local filesystem storage and S3-compatible cloud object storage. Each repository can be configured
with an independent storage backend.

## 1. Local Filesystem Storage

Configured via `storage_path` in `config.yaml` (default: `storage`).

### Layout Organization

- **Maven**: `{storage_path}/{repo}/{group_path}/{artifact}/{version}/{files}`
- **Cargo**: `{storage_path}/{repo}/crates/{crate}/{version}.crate`
- **Docker**: `{storage_path}/_docker/blobs/...` and `{storage_path}/_docker/manifests/...`

### Write Reliability

- Uploaded files are written to `.tmp` temporary files with checksum verification.
- Once verified, the file is atomically renamed into its final location.

---

## 2. S3-Compatible Object Storage

Suitable for multi-node deployments, container clusters, or distributed environments.

### Supported Providers

- **AWS S3**
- **MinIO** (Self-hosted)
- **Cloudflare R2**
- **Aliyun OSS / Tencent COS / Huawei OBS** (via S3 compatibility layer)

### Configuration Example (`repositories.yaml`)

```yaml
repositories:
  releases:
    name: releases
    s3:
      enabled: true
      endpoint: "https://minio.internal:9000"
      bucket: "renop-storage"
      key_prefix: "releases/"
      region: "us-east-1"
      access_key_id: "ACCESS_KEY"
      secret_access_key: "SECRET_KEY"
      force_path_style: true
      redirect_downloads: false
```

### Download Delivery Modes

1. **Proxy Streaming (`redirect_downloads: false`)**:
    - RenoP streams data from S3 to the client.
    - Ideal when the S3 bucket is private and not exposed to the public Internet.
2. **Direct Redirect (`redirect_downloads: true`)**:
    - RenoP authenticates the request and responds with a `302 Found` redirecting to a presigned S3 URL.
    - Reduces bandwidth overhead on the RenoP server.
