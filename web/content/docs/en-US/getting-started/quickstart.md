---
title: Quick start
order: 3
category: Getting started
description: First run, admin password, default repo URLs
---

# Quick start

## First start

First run creates an `admin` account. Set the password before starting:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

If unset, a random password is printed to the server log. Then open `http://localhost:3000`.

Log in as `admin`. Managers can use the web UI for artifacts, users, repos, and settings.

## Default repositories

| Path                              | Role      |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

Use these in Maven `<repositories>` / `<distributionManagement>`. Examples: [Maven client](./maven-client.md).

## Health check

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## Environment variables

| Variable                       | Default             | Purpose                          |
|--------------------------------|---------------------|----------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Server, frontend, storage, updater |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repos, mirrors, per-repo S3      |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Accounts and tokens              |
| `RENOP_INDEX`                  | `index.json`        | Artifact index                   |
| `RENOP_SESSIONS`               | `sessions.json`     | Login sessions                   |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | generated           | First admin password             |

Most of this is also editable in the management UI. Restart after changing listen address or TLS.

## Next

1. [Configuration](../configuration/overview.md) — bind, TLS, branding
2. [Repositories & mirrors](../configuration/repositories.md)
3. [Maven client](./maven-client.md)
4. [HTTP API](../api/README.md)
