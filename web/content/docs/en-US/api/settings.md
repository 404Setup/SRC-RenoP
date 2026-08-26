---
title: Settings API
order: 8
category: API Reference
description: Domain-based service settings, repository management, and index rebuilds
---

# Settings API

All settings routes require a manager account or an API token with `admin:settings` or `admin:repositories`, according
to the operation. Responses use protobuf where defined in `proto/api/v1/api.proto`.

## Discover setting domains

- **Path**: `GET /api/settings/domains`
- **Response**: Stable domain names currently supported by the server, including `server`, `proxy`, `storage`,
  `updater`, and `index`.

## Read and update one domain

- **Read**: `GET /api/settings/domain/:name`
- **Update**: `PUT /api/settings/domain/:name`
- **Behavior**: The request and response schema depends on `:name`. Unknown fields and invalid values are rejected.
  Host, port, TLS, database, and selected runtime changes may require a service restart.
- **GitHub OAuth**: `GET /api/settings/github-oauth` reads redacted state and `PUT /api/settings/github-oauth` updates the
  client ID and write-only secret.

## Repository settings

The generic `/api/settings/repositories` routes are preferred. Maven-prefixed aliases remain for compatibility.

### List repositories

- **Path**: `GET /api/settings/repositories`
- **Alias**: `GET /api/settings/maven/repositories`

### Create, update, delete, or migrate

- **Create or update**: `PUT /api/settings/repositories/:name`
- **Delete**: `DELETE /api/settings/repositories/:name`
- **Migrate Maven/files**: `POST /api/settings/repositories/:name/migrate/:target`, where `:target` is `maven` or
  `files`. Stored objects remain in place while the Maven catalog is rebuilt when returning to Maven.

## Rebuild the search index

- **Path**: `POST /api/settings/index/rebuild`
- **Behavior**: Submits a coalesced background rebuild. A concurrent rebuild is not started twice.
