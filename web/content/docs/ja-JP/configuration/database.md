---
title: データベース設定
order: 3
category: 設定
description: SQLite、MySQL、PostgreSQL の接続と connection pool
---

# データベース設定

RenoP はアカウント、RBAC、API Token、セッション、監査、チーム、メッセージをデータベースに永続化します。
`config.yaml` の `database` を設定してください。マイグレーションは起動時に自動適用されます。

## SQLite（既定）

SQLite は組み込みで、外部サービスを必要としません。

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

- `dsn` は相対または絶対ファイルパスです。
- RenoP が schema を初期化し、並行アクセス用に WAL を有効化します。

## MySQL 8.0+

外部管理データベースには MySQL を利用できます。

```yaml
database:
  driver: "mysql"
  dsn: "renop_user:password@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### MySQL 要件

- MySQL 8.0 以降を推奨します。
- `utf8mb4` と `utf8mb4_unicode_ci` または `utf8mb4_0900_ai_ci` を使用します。
- RenoP schema の table 作成と変更権限が必要です。

## PostgreSQL

PostgreSQL は `jackc/pgx/v5` driver を使用します。

```yaml
database:
  driver: "postgres"
  dsn: "postgres://renop_user:password@127.0.0.1:5432/renop_db?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### DSN 形式

- **URI**: `postgres://username:password@host:port/dbname?sslmode=disable`
- **Key-Value**: `host=127.0.0.1 port=5432 user=renop_user password=password dbname=renop_db sslmode=disable`

本番環境では `sslmode=disable` ではなく、データベース提供元の方針に従って TLS を有効化してください。

## ClickHouse 26.9+

RenoP はセルフ管理 ClickHouse に `clickhouse-go/v2` のネイティブ `clickhouse.Open` API で接続し、
`database/sql` 互換 API は使用しません。RenoP の起動前に専用 database を作成してください。

```yaml
database:
  driver: "clickhouse"
  dsn: "clickhouse://renop_user:password@127.0.0.1:9000/renop_db?compress=lz4"
  max_open_conns: 8
  max_idle_conns: 4
  conn_max_lifetime_sec: 600
```

公式 build に `EmbeddedRocksDB` が必要です。RenoP は衝突しない複合キーを materialized column として持つ
可変 key-value table を作成し、行更新を同期 native mutation に変換します。ClickHouse 26.9 には複数文の
transaction がないため、書き込みを直列化し、行単位の永続 snapshot を journal に記録します。中断した
transaction は次回起動時に復元されます。この保存モードは ClickHouse Cloud に対応しません。
ClickHouse 26.9.1 と `clickhouse-go/v2` 2.48.0 で matrix test 済みです。

## connection pool

| パラメーター            | 既定  | 説明                           |
|:------------------------|:------|:-------------------------------|
| `max_open_conns`        | `25`  | 開く接続数の上限               |
| `max_idle_conns`        | `25`  | idle 接続数の上限              |
| `conn_max_lifetime_sec` | `300` | 接続を再作成するまでの最大秒数 |

SQL サーバーの接続上限に合わせて設定します。過剰な値は throughput を改善せず、メモリと接続枠を消費します。
