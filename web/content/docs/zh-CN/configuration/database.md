---
title: 数据库配置
order: 3
category: 配置
description: SQLite、MySQL 与 PostgreSQL 数据库连接与连接池配置
---

# 数据库配置

RenoP 支持使用内嵌 SQLite、外部 MySQL 或 PostgreSQL 作为元数据与凭据的持久化数据库。数据库用于存储用户账户、角色权限、个人访问令牌（PAT）、会话状态、审计日志与消息中心数据。

相关配置位于 `config.yaml` 中的 `database` 节点。

## SQLite（默认配置）

默认情况下，RenoP 使用内置的 SQLite 驱动，无需安装任何独立数据库服务：

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

- `dsn` 可以是相对路径（如 `renop.db`，存放在程序当前工作目录下）或绝对路径。
- 程序首次启动时会自动创建表结构并开启 WAL 模式以提升并发性能。

## MySQL 8.0+

在多实例或团队统一管理环境下，可配置外部 MySQL 数据库：

```yaml
database:
  driver: "mysql"
  dsn: "renop_user:password@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### MySQL 准备要求

- MySQL 推荐版本为 8.0 或更高版本。
- 数据库字符集建议设置为 `utf8mb4`，排序规则设置为 `utf8mb4_unicode_ci` 或 `utf8mb4_0900_ai_ci`。
- RenoP 在连接成功后会自动检查并执行 Schema 迁移。

## PostgreSQL

RenoP 支持 PostgreSQL（基于 `jackc/pgx/v5` 驱动）：

```yaml
database:
  driver: "postgres"
  dsn: "postgres://renop_user:password@127.0.0.1:5432/renop_db?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### DSN 格式说明

PostgreSQL 支持标准 URI 格式或 Key-Value 格式连接串：

- **URI 格式**：`postgres://username:password@host:port/dbname?sslmode=disable`
- **Key-Value 格式**：`host=127.0.0.1 port=5432 user=renop_user password=your_password dbname=renop_db sslmode=disable`

## ClickHouse 26.9+

RenoP 通过 `clickhouse-go/v2` 的原生 `clickhouse.Open` API 连接自行维护的 ClickHouse，不使用
`database/sql` 兼容 API。需要在 RenoP 启动前创建独立数据库：

```yaml
database:
  driver: "clickhouse"
  dsn: "clickhouse://renop_user:password@127.0.0.1:9000/renop_db?compress=lz4"
  max_open_conns: 8
  max_idle_conns: 4
  conn_max_lifetime_sec: 600
```

官方 ClickHouse 构建必须包含 `EmbeddedRocksDB`。RenoP 使用物化的无碰撞复合键创建可变键值表，并把行更新
转换为同步原生 mutation。ClickHouse 26.9 不提供多语句事务，因此 RenoP 会串行执行写操作并记录行级持久
快照；中断的事务会在下次启动时根据日志回滚。此存储模式不支持 ClickHouse Cloud。数据库矩阵已使用
ClickHouse 26.9.1 与 `clickhouse-go/v2` 2.48.0 验证。

## 连接池参数说明

| 参数名                  | 默认值 | 作用说明                                                                 |
|:------------------------|:-------|:-------------------------------------------------------------------------|
| `max_open_conns`        | `25`   | 数据库最大打开连接数。高并发场景下可调大至 50-100。                      |
| `max_idle_conns`        | `25`   | 闲置连接池的最大连接数，通常建议设置为与 `max_open_conns` 接近或较小值。 |
| `conn_max_lifetime_sec` | `300`  | 连接的最大存活时间（秒）。超过该时间的空闲连接会被自动回收关闭。         |
