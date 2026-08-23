---
title: Quickstart
order: 3
category: Getting Started
description: Initial startup, administrator password setup, and default repository endpoints
---

# Quickstart

## 1. Initial Boot

Upon initial startup, RenoP automatically initializes the default security context and creates the super-administrator
account `admin`.

Set the administrator password beforehand via an environment variable:

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

If this environment variable is not set, RenoP will generate a random secure password and print it to stdout during
startup.

Once started, navigate to `http://localhost:3000` in your web browser.

## 2. Default Repository Endpoints

RenoP initializes the following default repositories:

| Endpoint URL                      | Visibility | Purpose                                                     |
|:----------------------------------|:-----------|:------------------------------------------------------------|
| `http://localhost:3000/releases`  | `PUBLIC`   | Maven release repository (redeployment disabled by default) |
| `http://localhost:3000/snapshots` | `PUBLIC`   | Maven snapshot repository (redeployment enabled)            |
| `http://localhost:3000/private`   | `PRIVATE`  | Maven private repository (authentication required)          |

Cargo and Docker endpoints are also ready out of the box:

- Cargo Index: `http://localhost:3000/index/` (or repository-specific paths)
- Docker Registry: `http://localhost:3000/v2/`

## 3. Health Probes

Verify that the service is running using the health probe endpoint:

```bash
curl -s http://localhost:3000/api/status/health
# Output: "UP"
```

## 4. Key Environment Variables

| Variable                       | Default Value       | Description                                            |
|:-------------------------------|:--------------------|:-------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Primary server configuration file                      |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repository and mirror configurations                   |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Initial users and static tokens (migrated to database) |
| `RENOP_INDEX`                  | `index.json`        | Search index cache file                                |
| `RENOP_SESSIONS`               | `sessions.bin`      | Binary session storage file                            |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | *(Generated)*       | Initial administrator password                         |

## 5. Next Steps

- [Configuration Overview](../configuration/overview.md) — Server settings, TLS, databases, and storage
- [Repositories & Mirrors](../configuration/repositories.md) — Custom repositories, caching rules, and S3 backends
- [Maven & Gradle Guide](../guides/maven-client.md) — Client integration for Java/Kotlin builds
- [Cargo Registry Guide](../guides/cargo-registry.md) — Rust / Cargo registry configuration
- [Docker Registry Guide](../guides/docker-registry.md) — Docker and Podman image management
