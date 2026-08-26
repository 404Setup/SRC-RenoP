---
title: Docker & OCI Registry
order: 3
category: Guides
description: Creating images and using Docker, Podman, containerd, or nerdctl with RenoP
---

# Docker & OCI Registry Guide

Create a repository with format `docker`, then create each target image in its repository before push. The examples use
repository `containers` and image `team/service`, producing registry name `containers/team/service`.

## Login and transport

```bash
docker login localhost:3000
# Username: admin
# Password: <your_password_or_API_token>
```

Use a dedicated API Token: `repository:read` for pull, `repository:publish` for push, `repository:delete` for remote
deletion, `package:create` for image reservation through the management API, and `team:manage` for collaborators. The
short-lived Docker token receives only actions allowed by both scopes/targets and the image's current L0-L4 policy.

Production registries should use HTTPS. For local HTTP testing only, configure Docker explicitly:

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

Restart the Docker daemon after changing `daemon.json`. Podman and containerd have equivalent registry trust settings.

## Create, tag, and push

Open repository `containers`, create image `team/service`, and choose public or private visibility. Private images grant
no implicit L0 access; add readers or collaborators from the image team. Names use lowercase path components.

Creation returns a conflict when the name exists locally or on an applicable enabled upstream. If the upstream check is
inconclusive, RenoP does not reserve the name. Mirror-discovered images remain pull-only.

```bash
# Tag local image
docker tag service:latest localhost:3000/containers/team/service:1.0.0

# Push image to RenoP
docker push localhost:3000/containers/team/service:1.0.0
```

RenoP rejects token push grants, blob upload initiation, and manifest publication until the image has been created.
Chunk retries remain valid after a failed management request; recreating the login or browser dialog is not required.

## Pull and run

```bash
# Pull image
docker pull localhost:3000/containers/team/service:1.0.0

# Run container
docker run -d -p 8080:8080 localhost:3000/containers/team/service:1.0.0
```

Public images are readable anonymously. Private images require an explicit L0-L4 member or administrator. Blob access
is image-scoped: possessing a digest from another image does not grant access.

## OCI behavior

- **Multi-architecture**: Manifest lists and OCI indexes can reference amd64, arm64, and other platforms.
- **Chunked uploads**: Large blobs support resumable POST/PATCH/PUT flows and bounded temporary storage.
- **Cross-repository mounts**: Mounts require read access to the source and write access to a pre-created destination.
- **Deletion**: Tag, manifest, and image deletion require both Token capability and image/repository authorization.
- **Mirrors**: Upstream responses are streamed and cataloged with origin metadata; mirrored images cannot be pushed.
