---
title: Repositories & Mirrors
order: 2
category: Configuration
description: repositories.yaml configuration, visibility levels, upstream mirrors, and S3 backends
---

# Repositories & Mirrors

Repository definitions are stored in `repositories.yaml` (overridden by `RENOP_REPOSITORIES`). Most settings can also be
modified in the Web console under "Repository Settings".

## Configuration Example

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false

  snapshots:
    name: snapshots
    visibility: PUBLIC
    allow_redeployment: true
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false

  private:
    name: private
    visibility: PRIVATE
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false
```

## Repository Fields

| Field                   | Type   | Default  | Description                                                            |
|:------------------------|:-------|:---------|:-----------------------------------------------------------------------|
| `name`                  | string | Required | Repository identifier and URL path prefix (`http://host:3000/{name}/`) |
| `visibility`            | string | `PUBLIC` | Visibility level: `PUBLIC`, `HIDDEN`, or `PRIVATE`                     |
| `allow_redeployment`    | bool   | `false`  | Allows overwriting existing artifact versions upon re-upload           |
| `require_gpg_signature` | bool   | `false`  | Enforces detached OpenPGP signatures before releasing artifacts        |
| `mirrors`               | list   | `[]`     | Upstream proxy mirror configurations                                   |
| `s3`                    | object | `{}`     | S3-compatible object storage backend configuration                     |

### Visibility Levels

- **PUBLIC**: Publicly readable. Anonymous users can download artifacts and view listings without authentication.
- **HIDDEN**: Unlisted but directly readable. User-facing repository catalogs and profile memberships omit the
  repository for every viewer; users who know an exact artifact URL can still download it. Administrators continue to
  see and configure the repository in repository management.
- **PRIVATE**: Private repository. Downloading, listing, and uploading all require valid credentials with appropriate
  permissions.

## Upstream Mirror Configuration (`mirrors`)

When a requested artifact is not present locally, RenoP can proxy the request to upstream repositories and optionally
cache files locally.

```yaml
mirrors:
  - name: "maven-central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

| Field             | Default  | Description                                                                |
|:------------------|:---------|:---------------------------------------------------------------------------|
| `name`            | Required | Mirror identifier                                                          |
| `url`             | Required | Base URL of upstream repository                                            |
| `persist`         | `true`   | Caches fetched artifacts to local storage                                  |
| `cache_ttl_secs`  | `86400`  | Cache retention duration in seconds                                        |
| `negative_cache`  | `true`   | Caches 404 responses to avoid repetitive upstream misses                   |
| `timeout_secs`    | `30`     | Upstream request timeout in seconds                                        |
| `proxy`           | `""`     | Empty = global proxy; `direct` = bypass proxy; or a named proxy identifier |
| `allow_artifacts` | `[]`     | Whitelist rules (e.g. `com.example`), proxies only matching coordinates    |
| `deny_artifacts`  | `[]`     | Blacklist rules, blocks proxying matching coordinates                      |

## S3-Compatible Object Storage (`s3`)

To store repository artifacts in AWS S3 or MinIO:

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

| Field                | Default  | Description                                                   |
|:---------------------|:---------|:--------------------------------------------------------------|
| `enabled`            | `false`  | Enables S3 storage for this repository                        |
| `endpoint`           | Required | S3 API endpoint URL                                           |
| `bucket`             | Required | Bucket name                                                   |
| `key_prefix`         | `""`     | Object key prefix inside the bucket (e.g. `releases/`)        |
| `region`             | `auto`   | S3 bucket region                                              |
| `access_key_id`      | Required | S3 Access Key ID                                              |
| `secret_access_key`  | Required | S3 Secret Access Key                                          |
| `force_path_style`   | `true`   | Uses path-style URLs (required for MinIO)                     |
| `redirect_downloads` | `false`  | Issues 302 redirects to presigned S3 URLs instead of proxying |
