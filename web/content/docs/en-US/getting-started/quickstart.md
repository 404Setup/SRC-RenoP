---
title: Quick start
order: 3
category: Getting started
description: First run, admin password, default repository URLs
---

# Quick start

## First start

On first startup, RenoP creates an `admin` account. Set its password with an environment variable before starting the
process:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

If the variable is not set, a random password is generated and written to the server log. After startup, open
`http://localhost:3000`.

Sign in as `admin`. Accounts with manager or admin permissions can manage artifacts, users, repositories, and settings
in the web UI.

## Default repositories

| Path                              | Role      |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

Configure these URLs in Maven `<repositories>` or `<distributionManagement>`. See [Maven client](./maven-client.md) for
examples.

## Health check

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## Environment variables

| Variable                       | Default             | Purpose                                                       |
|--------------------------------|---------------------|---------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Server, frontend, storage, updater                            |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repositories, mirrors, per-repository S3                      |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Accounts and tokens                                           |
| `RENOP_INDEX`                  | `index.json`        | Artifact index                                                |
| `RENOP_SESSIONS`               | `sessions.bin`      | Login sessions (protobuf; legacy `sessions.json` is migrated) |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | generated           | Password for the first admin account                          |

Most settings can also be changed in the management UI. Restart the process after changing the listen address or TLS
settings.

## Next steps

1. [Configuration](../configuration/overview.md) — bind address, TLS, branding
2. [Repositories & mirrors](../configuration/repositories.md)
3. [Maven client](./maven-client.md)
4. [HTTP API](../api/README.md)
