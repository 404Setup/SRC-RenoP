---
title: Introduction
order: 1
category: Getting Started
description: Overview of RenoP multi-protocol package repository server
---

# Introduction to RenoP

RenoP is a self-hosted multi-protocol package and artifact repository server. Written in Go with an embedded single-page
Web management interface, RenoP provides a lightweight, dependency-free, and easy-to-operate solution for private
artifact hosting.

## Supported Protocols & Ecosystems

- **Maven / Gradle**: Supports Release, Snapshot, and Private repositories following standard Maven repository layouts,
  complete with Javadoc preview and GPG signature verification.
- **Cargo (Rust)**: Supports the modern Cargo Sparse Index protocol (`sparse+http(s)://`), crate publishing,
  downloading, search, yanking, crates.io mirror proxying, and Cargodoc online documentation viewing.
- **Docker / OCI Registry**: Complies with OCI Distribution Spec v2 and Docker Registry v2, supporting
  multi-architecture manifest lists, chunked blob uploads, and upstream registry mirrors.

## Storage & Database Backends

- **Storage**: Local filesystem storage or S3-compatible object storage (AWS S3, MinIO, Cloudflare R2, Aliyun OSS,
  Tencent COS).
- **Database**: Embedded SQLite by default, with native support for external MySQL 8.0+ and PostgreSQL.

## Core Features

| Feature                  | Description                                                                                                    |
|:-------------------------|:---------------------------------------------------------------------------------------------------------------|
| **Single Binary**        | Zero runtime dependencies; includes the embedded Web UI for instant startup                                    |
| **Upstream Mirroring**   | Transparent proxying for Maven, Cargo, and Docker with positive/negative caching and coordinate rules          |
| **Granular RBAC**        | Role-based access control, repository-level permissions (read/write/admin), and Personal Access Tokens         |
| **Daemon Lifecycle**     | Built-in `--install` and `--uninstall` commands for Windows Services, systemd, OpenRC, LaunchDaemons, and rc.d |
| **Security & Hardening** | Detached OpenPGP signature verification, sliding-window rate limiting, and anomaly IP bans                     |

## Documentation Navigation

- [Installation Guide](./install.md) — Prebuilt binaries, microarchitecture tiers, and source compilation
- [Quickstart](./quickstart.md) — Bootstrapping, admin credentials, and default endpoints
- [System Architecture](./architecture.md) — Internal modules and request lifecycle
- [Configuration Overview](../configuration/overview.md) — Configuration files and environment variables
- [Maven & Gradle Guide](../guides/maven-client.md) — Client integration for Maven and Gradle
- [Cargo Registry Guide](../guides/cargo-registry.md) — Rust / Cargo registry configuration
- [Docker Registry Guide](../guides/docker-registry.md) — Docker and Podman integration
