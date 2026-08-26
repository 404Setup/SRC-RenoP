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
# Password: <your_password_or_API_token>
```

Use `repository:read` for pulls, `repository:publish` for pushes, `repository:delete` for remote manifest/blob deletion,
and `package:manage` for image-team operations. The short-lived Docker Bearer token receives only actions allowed by
both these API-token scopes and the image team.

> **Note**: When running over plain HTTP (without TLS), add the host to your Docker `daemon.json`:
> ```json
> {
>   "insecure-registries": ["localhost:3000"]
> }
> ```

## 2. Create, Tag, and Push Images

Open the Docker repository in RenoP and create the target image first. Choose public or private visibility during
creation. A private image grants no implicit public access; add L0 readers or higher-level collaborators from its team
panel. Image names use lowercase path components such as `team/service`.
The name must be unique within the repository. When upstream mirrors are enabled, RenoP also checks every applicable
mirror and rejects names that already exist upstream. Creation is temporarily unavailable if that check cannot produce
an authoritative result.

```bash
# 1. Create my-app in the my-org repository through the RenoP UI

# 2. Tag local image
docker tag my-app:latest localhost:3000/my-org/my-app:1.0.0

# 3. Push image to RenoP
docker push localhost:3000/my-org/my-app:1.0.0
```

RenoP rejects token push scope, blob upload initiation, and manifest publication when the target image has not been
created. Existing upstream mirror pulls remain available and use a separate mirror-cache import path. Mirror-discovered
images remain pull-only; their names cannot be reused for a local push image.

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
