---
title: Introduction
order: 1
category: Getting Started
description: RenoP as an integrated self-hosted package publication platform
---

# Introduction to RenoP

RenoP is an integrated, self-hosted package publication and distribution server. Its model is closer to a private
Central-style service than to a Maven-only repository: one Go process embeds the management UI, identity, teams,
verification workflows, package catalogs, mirrors, storage, audit, and updates.

## Supported protocols

- **Maven / Gradle**: Verified global publishing domains, modern domain catalogs, classic layout compatibility, Maven 2
  client paths, mirrors, Javadoc, and detached OpenPGP verification.
- **Cargo**: Sparse Index, explicit package ownership, publication, search, yank/unyank, mirrors, and Cargodoc.
- **Docker / OCI**: Distribution v2, explicit image reservation, private image teams, chunked blobs, cross-repository
  mounts, multi-architecture manifests, and upstream mirrors.
- **Files**: Unstructured replaceable file storage with mirrors and no generated Maven metadata or signature workflow.

## Storage and databases

- **Storage**: Streaming local Disk or repository-specific S3-compatible object storage.
- **Database**: Embedded SQLite by default, with external MySQL and PostgreSQL support.
- **Consistency**: Repository gates coordinate uploads, deletes, mirror commits, GPG publication, and engine/storage
  changes without reading large objects fully into memory.

## Core capabilities

| Capability | Description |
|:-----------|:------------|
| **Single service** | Embedded frontend and protocol APIs with no separate application runtime |
| **Global identity** | Username-based public profiles backed by immutable internal user IDs |
| **Granular access** | Repository permissions, L0-L4 package/domain teams, scoped and expiring API tokens |
| **Verified publishing** | Maven domain ownership, upstream name-conflict checks, and optional OpenPGP quarantine |
| **Operations** | Native service installation, scheduled maintenance, durable audit/messages, and in-place updates |
| **Defense** | Bounded streaming, rate limits, anomaly bans, trusted-proxy validation, and sandboxed documentation viewers |

## Documentation map

- [Installation](./install.md) — Release packages, platform selection, and source builds
- [Quickstart](./quickstart.md) — First startup, administrator bootstrap, and repository creation
- [Architecture](./architecture.md) — Modules, authorization, storage, and asynchronous work
- [Configuration](../configuration/overview.md) — Validated settings and environment overrides
- [Maven & Gradle](../guides/maven-client.md) — Verified domains and JVM client setup
- [Cargo](../guides/cargo-registry.md) — Sparse registry and crate lifecycle
- [Docker & OCI](../guides/docker-registry.md) — Image reservation, authentication, push, and pull
