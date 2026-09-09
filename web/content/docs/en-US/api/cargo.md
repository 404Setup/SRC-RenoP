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

## Resource Locks

Administrators and this repository's moderators use `PUT /{repo}/api/v1/crates/{crate_name}/locks` with an
active browser session cookie. API tokens and package credentials cannot manage locks.

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

Omit `version` or use `""` to lock the package. Modes are `write` and `read`. Public, localized reasons are
`hold`, `prohibited`, `expired`, `trojan`, `abuse`, `dmca`, `reup`, `squatting`, and `quality`.
Successful changes return `{"ok":true}`. To remove the exact manual lock, send
`DELETE /{repo}/api/v1/crates/{crate_name}/locks` with `{"version":"1.2.3"}`.

Write locks freeze mutations and upstream refreshes while keeping stored downloads available. Read locks also block
every file download and documentation preview, including for staff. Metadata remains visible to administrators,
repository moderators, owners, and collaborators, including L0 members and bound global-team members. Other viewers
cannot see the locked package or version in metadata, sparse indexes, search, profiles, or team resource lists.

Package and version metadata expose public `locks` records with `mode`, `reason`, `source`, and `locked_at`.
System and manual locks are independent; removing a manual lock preserves system restrictions. Mutations return
`423` with `X-Renop-Error-Code: resource_locked`; denied reads return `404`. A locked version prevents whole-package
archive, deprecation, and deletion, but other versions can still be published. Repository reconfiguration and deletion
are blocked while resource locks exist.
