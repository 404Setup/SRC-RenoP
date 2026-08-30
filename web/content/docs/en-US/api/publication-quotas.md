---
title: Publication Quotas
order: 18
category: API Reference
description: Periodic publication limits for accounts and global teams
---

# Publication Quotas

Publication quotas bound locally uploaded files, bytes, and completed project publications. New installations default to
600 files, 40 MiB, and 20 publications per month. A system administrator can change the global defaults or define an
account- or global-team-specific override.

## Policy

The `period` is `day`, `week`, or `month`; boundaries use UTC. A limit of zero is valid only for an owner override and
prevents that operation. The administrator-only `unlimited` override disables quota consumption for that owner. An empty
override object restores every global default.

## Ownership

A personally owned package consumes the publishing account's quota. A package or Maven publishing domain bound to a
global team consumes only that team's quota. Transferring ownership changes the quota owner for future publications; it
does not move historical usage. Mirror downloads and mirror catalog updates never consume publication quota.

## Usage Accounting

Cargo and npm count one stored package file and one completed publication per accepted version. Docker counts the
manifest, config, and layer descriptors and completes one publication at manifest submission. Maven counts every client
PUT as a file and counts a project publication when its POM is accepted. The unstructured files engine counts each PUT as
one file and one publication. Server-generated indexes and checksums do not add separate usage.

Concurrent uploads first create a durable, expiring reservation. A successful protocol validation commits it; rejected
or abandoned reservations are released or removed by scheduled cleanup. Current status includes committed usage and live
reservations so parallel requests cannot exceed a limit.

## Endpoints

```http
GET /api/publication-quota
GET /api/publication-quota/users/{username}
PUT /api/publication-quota/users/{username}
GET /api/publication-quota/super-teams/{prefix}
PUT /api/publication-quota/super-teams/{prefix}
GET /api/settings/publication-quota
PUT /api/settings/publication-quota
```

The current account may read its own status. Global-team members may read their team's status. Only a system
administrator may read another account, change overrides, use `unlimited`, or update global defaults.

## Enforcement

An exhausted quota returns `429 Too Many Requests`. `X-Renop-Error-Code` distinguishes
`publication_file_quota`, `publication_byte_quota`, and `publication_count_quota`. Quotas apply after authentication,
repository permission, package reservation, namespace binding, and Maven-domain verification checks; they never grant
permission that the account does not already hold.
