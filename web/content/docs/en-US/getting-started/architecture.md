---
title: System Architecture
order: 4
category: Getting Started
description: Internal modular architecture, streaming I/O, and data flow
---

# System Architecture

This document describes the internal modular architecture and request processing pipeline of RenoP.

## 1. Modular Subsystems

RenoP is structured into five cohesive subsystems:

```
+-------------------------------------------------------------------+
|                     Embedded Web Management UI                    |
|          (Single-page app, multi-locale i18n, dark/light theme)   |
+-------------------------------------------------------------------+
                                  │
                                  ▼
+-------------------------------------------------------------------+
|                      HTTP Routing & Middleware                    |
|        - Anomaly detection & sliding-window rate limiting         |
|        - Unified authentication (Cookie session / Bearer / Basic) |
|        - Dual serialization (JSON / Protobuf binary support)      |
+-------------------------------------------------------------------+
                                  │
                                  ▼
+-------------------------------------------------------------------+
|                       Service Domain Layer                        |
|  - Maven service (standard layouts, Javadoc extraction, GPG check)|
|  - Cargo service (sparse index, crate publish/download, Cargodoc) |
|  - Docker service (OCI v2 spec, chunked uploads, blob mounting)   |
|  - Management services (users, tokens, settings, message center)  |
+-------------------------------------------------------------------+
                                  │
                 ┌────────────────┴────────────────┐
                 ▼                                 ▼
+---------------------------------+  +-------------------------------+
|     Database Abstraction Layer  |  |   Storage Abstraction Layer   |
| - SQLite (embedded default)     |  | - Local filesystem            |
| - MySQL 8.0+                    |  | - S3-compatible (MinIO, R2)   |
| - PostgreSQL (via pgx/v5)       |  | - GPG quarantine queue        |
+---------------------------------+  +-------------------------------+
```

## 2. Request Processing & I/O Pipeline

### Streaming Data Transfer

For large artifact uploads and downloads (fat JARs, crate archives, Docker image layer blobs):

- Data streams directly between the client connection and the storage engine (disk or S3) without loading entire files
  into memory.
- Low-level byte buffers are pooled to minimize heap allocations and garbage collection overhead.

### Unified Database Layer

RenoP supports SQLite, MySQL, and PostgreSQL:

- User credentials, access tokens, audit logs, and message center items are persisted in the configured database.
- Uses parameterized queries with automated placeholder rebinding for PostgreSQL.

### Storage Consistency & Quarantine Queues

- **Local Disk Writes**: Files are written to `.tmp` temporary files with checksum verification, then atomically moved
  to their destination path upon completion.
- **GPG Quarantine**: Repositories requiring GPG verification hold unsigned artifacts in `.renop.tmp.gpg` until the
  detached signature (`.asc`) is validated.
