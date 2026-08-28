---
title: Repositories & Mirrors
order: 2
category: Configuration
description: Repository engines, visibility, upstream mirrors, migration, and S3 storage
---

# Repositories & Mirrors

Repository definitions live in `repositories.yaml`, overridden by `RENOP_REPOSITORIES`. Administrators can edit the
same validated settings from repository management. Repository names are immutable lowercase slugs and form the first
URL path segment.

## Configuration example

```yaml
repositories:
  releases:
    name: releases
    format: maven
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: true
    download_statistics: true
    mirrors: []
  crates:
    name: crates
    format: cargo
    visibility: PUBLIC
    mirrors: []
  containers:
    name: containers
    format: docker
    visibility: PRIVATE
    allow_redeployment: false
    mirrors: []
```

## Repository fields

| Field | Default | Description |
|:------|:--------|:------------|
| `name` | Required | Immutable repository slug and URL prefix |
| `format` | `maven` | `maven`, `maven-classic`, `files`, `npm`, `cargo`, or `docker` |
| `visibility` | `PUBLIC` | `PUBLIC`, `HIDDEN`, or `PRIVATE` |
| `allow_redeployment` | `false` | Maven version redeployment or replacement in files/Docker, when supported |
| `require_gpg_signature` | `false` | Require detached OpenPGP validation for Maven publication |
| `download_statistics` | Engine default | Enabled for Maven/npm/Cargo/Docker; unstructured `files` opts in |
| `mirrors` | `[]` | Ordered upstream definitions |
| `s3` | omitted | Repository-specific S3-compatible storage |

`maven-classic` changes only the frontend layout and retains Maven publication rules. `files` is unstructured and does
not generate checksums, POM files, or signature validation. Maven repositories can migrate to `files` and back without
moving objects; returning to Maven rebuilds the catalog and restores saved Maven policy. Migration preserves the
repository's effective download-statistics switch.

An `npm` repository requires package reservation before publication, stores immutable semantic versions and dist-tags,
supports scoped private packages and L0-L4 teams, and can mirror exact package names or `@scope/*` patterns.

### Visibility

- **PUBLIC**: Anonymous reads and discovery are allowed.
- **HIDDEN**: Anonymous and unprivileged catalogs omit the repository, and profile memberships remain unlisted.
  Managers and viewers with explicit repository-browser permission can discover it. Exact known file paths remain
  readable.
- **PRIVATE**: Reads, listings, and writes require explicit authorization. Private Docker images additionally enforce
  image-level L0-L4 membership.

## Upstream mirrors

When a local object is missing, RenoP may stream it from an ordered enabled mirror. Successful fetches can be persisted
without buffering the whole body. Cargo and Docker prevent local creation when an applicable upstream name exists.

```yaml
mirrors:
  - name: "central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

| Field | Default | Description |
|:------|:--------|:------------|
| `name` | Required | Unique mirror name within the repository |
| `url` | Required | Upstream base URL |
| `persist` | `true` | Store successful responses in the repository backend |
| `cache_ttl_secs` | `86400` | Positive-cache lifetime |
| `negative_cache` | `true` | Cache supported upstream misses |
| `timeout_secs` | `30` | Per-request upstream timeout |
| `proxy` | `""` | Global route; `direct`; or an exact named proxy |
| `allow_artifacts` | `[]` | Format-aware allow rules |
| `deny_artifacts` | `[]` | Format-aware deny rules; deny wins |

Mirror credentials, when required, use the structured authorization fields. Do not embed secrets in `url`.

## S3-compatible storage

Each repository may use Disk or an independent S3-compatible backend. Changing the storage or engine is serialized
with active uploads, deletes, GPG commits, and mirror writes by the repository gate.

```yaml
s3:
  enabled: true
  endpoint: "https://s3.us-east-1.amazonaws.com"
  bucket: "my-renop-bucket"
  key_prefix: "releases/"
  region: "us-east-1"
  access_key_id: "YOUR_ACCESS_KEY"
  secret_access_key: "YOUR_SECRET_KEY"
  force_path_style: false
  redirect_downloads: false
```

`force_path_style` is commonly required by MinIO. With `redirect_downloads: true`, RenoP authorizes the request and
returns a short-lived presigned redirect; otherwise it streams the object through the server.
