---
title: Docker & OCI Registry
order: 3
category: Guides
description: Using Docker, Podman, and containerd to push and pull OCI images
---

# Docker & OCI Registry Guide

RenoP implements the OCI Distribution Spec v2 and Docker Registry v2, serving as a private container image registry
compatible with Docker CLI, Podman, containerd, and nerdctl.

## 1. Registry Login

Log in using `docker login` or `podman login`:

```bash
docker login localhost:3000
# Username: admin
# Password: <your_password_or_PAT>
```

> **Note**: When running over plain HTTP (without TLS), add the host to your Docker `daemon.json`:
> ```json
> {
>   "insecure-registries": ["localhost:3000"]
> }
> ```

## 2. Tag & Push Images

```bash
# 1. Tag local image
docker tag my-app:latest localhost:3000/my-org/my-app:1.0.0

# 2. Push image to RenoP
docker push localhost:3000/my-org/my-app:1.0.0
```

## 3. Pull & Run Images

```bash
# Pull image
docker pull localhost:3000/my-org/my-app:1.0.0

# Run container
docker run -d -p 8080:8080 localhost:3000/my-org/my-app:1.0.0
```

## 4. Supported OCI Capabilities

- **Multi-Architecture Manifests**: Push and pull multi-arch manifest lists across `linux/amd64`, `linux/arm64`, etc.
- **Chunked Blob Uploads**: Large layer blobs are streamed in chunks with resume support.
- **Cross-Repository Blob Mounting**: Common base layers are reused automatically across repositories without
  re-uploading.
