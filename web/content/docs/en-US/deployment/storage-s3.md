---
title: Storage Architecture
order: 3
category: Deployment
description: Local filesystem and per-repository S3-compatible object backends
---

# Storage Architecture

RenoP supports local Disk and S3-compatible object services. Each repository selects its backend; the repository gate
serializes backend changes with active operations.

## 1. Local filesystem

The root is `storage_path` in `config.yaml`, defaulting to `storage`.

### Organization

- **Maven/files**: `{storage_path}/{repo}/{path}`
- **Cargo**: Index and archive data remain isolated under the repository directory
- **Docker**: Blobs, manifests, and references are isolated and validated per image

Physical names are implementation details. Use protocol APIs instead of modifying the directory directly.

### Write reliability

- Uploads use bounded temporary files and validate size, hash, and policy before commit.
- Final publication is atomic when the filesystem supports it.
- Mirror commits, deletes, migrations, and GPG publications synchronize with backend changes.

---

## 2. S3-compatible object storage

S3 is suitable for managed object storage. Multi-node operation also requires an external database and coordination
consistent with RenoP's guarantees; S3 alone does not turn one process into a cluster.

### Providers

- **AWS S3**
- **MinIO**
- **Cloudflare R2**
- Any service implementing the required S3 API

### Example (`repositories.yaml`)

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

Create the bucket first. Credentials need read, write, list, and delete access below `key_prefix`. Use TLS and a secret
manager; never commit access keys to Git.

### Download modes

1. **Proxy streaming (`redirect_downloads: false`)**: RenoP authorizes and streams S3 data to the client. The bucket can
   remain private and its URL is not exposed.
2. **Direct redirect (`redirect_downloads: true`)**: RenoP authorizes and returns `302 Found` to a short-lived presigned
   URL, reducing RenoP bandwidth.
