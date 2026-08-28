---
title: System Architecture
order: 4
category: Getting Started
description: Modular services, authorization, streaming storage, and asynchronous work
---

# System Architecture

RenoP is one Go process with explicit boundaries between transport, package protocols, authorization, persistence, and
background maintenance. The embedded frontend calls the same bounded APIs available to external clients.

## Module boundaries

```text
Browser and package clients
        |
HTTP routing, rate limits, authentication, API-token policy
        |
Maven | npm | Cargo | Docker | Files | Management services
        |
Repository gate and publication workflows
        |
Disk or S3 storage          SQL database
        |                       |
File index and mirrors      Identity, teams, audit, messages
```

- `internal/api` and middleware own general HTTP contracts, search, anomaly detection, and credential boundaries.
- Format services own Maven domains/catalogs, npm packuments, Cargo Sparse Index, Docker Distribution v2, and viewers.
- The database layer supplies dialect-aware transactions for SQLite, MySQL, and PostgreSQL.
- Disk/S3 storage streams large bodies and the file index provides bounded metadata traversal.

## Request and work pipelines

### Streaming and consistency

Uploads and downloads stream between the client and Disk/S3. Hashing, Brotli/ZIP extraction, mirror caching, and GPG
publication use bounded readers and temporary files. A striped repository gate prevents storage or engine changes from
racing uploads, deletes, mirror commits, or final publication.

### Authentication and authorization

Browser sessions are cookie-only. Basic credentials are limited to standard package protocols. Bearer API Token scopes
and exact target restrictions are intersected with the account's current repository permission and L0-L4 package/domain
membership on every request. Immutable user IDs preserve ownership across username changes.

### Asynchronous work

One process-wide non-reentrant scheduler coalesces status snapshots, cleanup, index persistence, download-count flushes,
and update checks. Ordering-sensitive queues such as audit persistence, GPG publication, token mutations, and file
watching remain dedicated serial workers. Durable workflow results go to the message center; transient progress uses UI
state or toasts.
