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
- **GitHub OAuth**: `GET /api/settings/github-oauth` reads redacted state and `PUT /api/settings/github-oauth` updates
  the
  client ID and write-only secret.

**Other OAuth providers**: `GET /api/settings/oauth-providers` returns redacted clients and presets; `PUT /api/settings/oauth-providers` replaces the list. A `providers` array is required, and an explicit empty array removes all configured clients. Up to 32 clients and a 128 KiB request are supported. See [Third-party Login](../security/oauth-login.md) for credentials, provider presets, and account binding.

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

## Publishing-domain reservation

`GET /api/settings/maven-domains` and `PUT /api/settings/maven-domains` use JSON.
Discovery includes `maven_domains`. The default is:

```json
{"release_value":2,"release_unit":"year"}
```

`release_value` is an integer from 1 to 100. `release_unit` accepts `month` or `year`, using UTC calendar arithmetic.
The configuration file stores the same fields under `maven_domains`. A saved change applies to newly created security
locks without changing existing release dates or the separate 31-day voluntary closure period.
See [Maven domain health](maven.md).
