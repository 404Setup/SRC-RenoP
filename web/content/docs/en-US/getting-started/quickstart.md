---
title: Quick start
order: 3
category: Getting started
description: First run, admin password, and default repository URLs
---

# Quick start

## First start

On first start, RenoP creates an `admin` account. Set its password before starting the server:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

If the variable is not set, a random password is printed to the server log. Open `http://localhost:3000` after startup.

Sign in with username `admin` and that password. Manager accounts can open the web UI to browse artifacts, manage users,
repositories, and settings.

## Default repositories

| Path                              | Role               |
|-----------------------------------|--------------------|
| `http://localhost:3000/releases`  | Release artifacts  |
| `http://localhost:3000/snapshots` | Snapshot artifacts |
| `http://localhost:3000/private`   | Private artifacts  |

Use these URLs in Maven's `<repositories>` or `<distributionManagement>`. Full client
examples: [Maven client setup](./maven-client.md).

## Health check

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## Environment variables

| Variable                       | Default             | Purpose                                 |
|--------------------------------|---------------------|-----------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Server, frontend, storage path, updater |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repositories, mirrors, per-repo S3      |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Accounts and access tokens              |
| `RENOP_INDEX`                  | `index.json`        | Persisted artifact index                |
| `RENOP_SESSIONS`               | `sessions.json`     | Persisted login sessions                |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | generated           | Password for the first admin account    |

Most settings can also be changed from the management UI. Restart the server after changing listener or TLS settings.

## Next steps

1. [Configure](../configuration/overview.md) bind address, TLS, and branding
2. Define [repositories & mirrors](../configuration/repositories.md)
3. Wire [Maven clients](./maven-client.md)
4. Explore the [HTTP API](../api/README.md)
