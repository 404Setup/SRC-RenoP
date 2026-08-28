---
title: Cargo Registry API
order: 5
category: API Reference
description: Cargo Sparse Index endpoints, crate publishing, downloading, and yanking
---

# Cargo Registry API

RenoP implements the Cargo Registry and Sparse Index specifications.

## Sparse Index Configuration (`config.json`)

- **Path**: `GET /{repo}/config.json` or `GET /{repo}/index/config.json`
- **Description**: Read by Cargo on initial registry connection to discover endpoints.

### Response (JSON)

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## Sparse Index Metadata

- **Path**: `GET /{repo}/index/{prefix}/{crate_name}`
- **Description**: Returns line-delimited JSON crate metadata following standard Cargo index sharding rules.

---

## Publish Crate

- **Path**: `PUT /{repo}/api/v1/crates/new`
- **Auth**: Token required (`Authorization: <token>`)
- **Body**: 4-byte JSON length header + JSON metadata + `.crate` tarball binary payload.
- **Name conflicts**: A first publication returns `409 Conflict` when the normalized name exists locally or on an
  applicable enabled mirror. An inconclusive upstream check returns `503 Service Unavailable`.

For local publications, RenoP reads the `package.readme` declaration from the validated `Cargo.toml` and extracts that
file from the archive without buffering the crate. Package-detail responses expose at most 512 KiB of Markdown, which
the browser renders through the shared element and URL allowlist. Catalog and search pages do not load README bodies.

---

## Download Crate

- **Path**: `GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **Response**: `.crate` binary archive (`application/x-tar`).

---

## Yank & Unyank

- **Yank**: `DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank**: `PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **Auth**: Crate owner or Admin
