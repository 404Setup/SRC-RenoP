---
title: Quickstart
order: 3
category: Getting Started
description: First startup, administrator bootstrap, health checks, and repository creation
---

# Quickstart

## Start the server

On the first startup, RenoP creates the `admin` super-administrator in the database. Set its password explicitly:

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

Without the variable, RenoP generates a random password and prints it once to stdout. Store it immediately, then open
`http://localhost:3000`. The service binds `0.0.0.0:3000` by default; use TLS or a trusted reverse proxy in production.

## Default and new repositories

The initial `repositories.yaml` contains three backward-compatible Maven repositories:

| Path         | Visibility | Policy                         |
|:-------------|:-----------|:-------------------------------|
| `/releases`  | `PUBLIC`   | Maven, redeployment disabled   |
| `/snapshots` | `PUBLIC`   | Maven, redeployment enabled    |
| `/private`   | `PRIVATE`  | Maven, authentication required |

Create npm, Cargo, Docker, or `files` repositories explicitly from repository management. Docker images and npm
packages must be reserved from their repository page before clients can push. Cargo names are created only after the
upstream name check succeeds. Maven publication additionally requires a verified domain from the account menu.

## Verify health

```bash
curl -s http://localhost:3000/api/status/health
# Output: "UP"
```

Use `/api/status/instance` for protobuf runtime metrics. A successful health probe confirms only that the process is
serving; verify the database and configured storage with a real authenticated operation before accepting production
traffic.

## Important environment variables

| Variable                       | Default             | Purpose                                                  |
|:-------------------------------|:--------------------|:---------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Main configuration path                                  |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repository configuration path                            |
| `RENOP_INDEX`                  | `index.json`        | Persisted file-index snapshot path                       |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | Generated once      | Initial `admin` password when the account does not exist |

Accounts, sessions, teams, API tokens, audit logs, and messages are database data and have no YAML path variables.

## Next steps

- [Configuration Overview](../configuration/overview.md) — TLS, database, proxy, previews, and updater
- [Repositories & Mirrors](../configuration/repositories.md) — Engines, visibility, upstreams, migration, and S3
- [Maven & Gradle](../guides/maven-client.md) — Verify a publishing domain and configure JVM clients
- [Cargo Registry](../guides/cargo-registry.md) — Create a Cargo repository and publish crates
- [Docker Registry](../guides/docker-registry.md) — Create images before push and configure Docker or Podman
- [npm Registry](../guides/npm-registry.md) — Reserve packages and configure npm-compatible clients
