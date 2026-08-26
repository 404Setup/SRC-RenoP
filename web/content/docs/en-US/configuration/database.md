---
title: Database Configuration
order: 3
category: Configuration
description: SQLite, MySQL, and PostgreSQL database connections and connection pooling
---

# Database Configuration

RenoP uses a database backend to persist user accounts, RBAC permissions, Personal Access Tokens (PATs), browser
sessions, audit logs, and message center notifications.

Configure database options under the `database` section in `config.yaml`.

## SQLite (Default)

RenoP uses an embedded SQLite database out of the box with zero external configuration:

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

- `dsn` can be a relative or absolute path to the database file.
- Automatically initializes schema tables and enables WAL mode for high concurrency.

## MySQL 8.0+

For multi-instance deployments or enterprise environments:

```yaml
database:
  driver: "mysql"
  dsn: "renop_user:password@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### MySQL Requirements

- MySQL 8.0 or newer is recommended.
- Database charset should be `utf8mb4` with collation `utf8mb4_unicode_ci` or `utf8mb4_0900_ai_ci`.
- Schema migrations run automatically upon initial connection.

## PostgreSQL

RenoP supports PostgreSQL via the `jackc/pgx/v5` driver:

```yaml
database:
  driver: "postgres"
  dsn: "postgres://renop_user:password@127.0.0.1:5432/renop_db?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### DSN Formats

PostgreSQL supports URI format or Key-Value format:

- **URI**: `postgres://username:password@host:port/dbname?sslmode=disable`
- **Key-Value**: `host=127.0.0.1 port=5432 user=renop_user password=password dbname=renop_db sslmode=disable`

## Connection Pool Parameters

| Parameter               | Default | Description                                                     |
|:------------------------|:--------|:----------------------------------------------------------------|
| `max_open_conns`        | `25`    | Maximum number of open database connections                     |
| `max_idle_conns`        | `25`    | Maximum number of idle connections in the pool                  |
| `conn_max_lifetime_sec` | `300`   | Maximum lifetime (seconds) before idle connections are recycled |
