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

## ClickHouse 26.9+

RenoP supports self-managed ClickHouse through the native `clickhouse.Open` API from `clickhouse-go/v2`; it never
uses the slower `database/sql` compatibility API. Use an isolated database created before RenoP starts:

```yaml
database:
  driver: "clickhouse"
  dsn: "clickhouse://renop_user:password@127.0.0.1:9000/renop_db?compress=lz4"
  max_open_conns: 8
  max_idle_conns: 4
  conn_max_lifetime_sec: 600
```

The official ClickHouse build must include `EmbeddedRocksDB`. RenoP creates mutable key-value tables with materialized
collision-free composite keys and translates row updates to synchronous native mutations. Because ClickHouse 26.9 has
no multi-statement transactions, RenoP serializes writes and records row-level persistent snapshots; an interrupted
transaction is rolled back from its journal at the next startup. ClickHouse Cloud is not supported by this storage
mode. The implementation and driver matrix are tested against ClickHouse 26.9.1 and `clickhouse-go/v2` 2.48.0.

## Connection Pool Parameters

| Parameter               | Default | Description                                                     |
|:------------------------|:--------|:----------------------------------------------------------------|
| `max_open_conns`        | `25`    | Maximum number of open database connections                     |
| `max_idle_conns`        | `25`    | Maximum number of idle connections in the pool                  |
| `conn_max_lifetime_sec` | `300`   | Maximum lifetime (seconds) before idle connections are recycled |
