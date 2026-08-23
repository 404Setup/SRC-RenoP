---
title: データベース設定
order: 3
category: 設定
description: SQLite、MySQL、PostgreSQL の接続設定とコネクションプール
---

# データベース設定

RenoP はユーザーアカウント、権限、トークン、監査ログ、メッセージの保存にデータベースを使用します。

## 1. SQLite（標準）

```yaml
database:
  enabled: true
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

## 2. MySQL 8.0+

```yaml
database:
  enabled: true
  driver: "mysql"
  dsn: "renop_user:password@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

## 3. PostgreSQL

```yaml
database:
  enabled: true
  driver: "postgres"
  dsn: "postgres://renop_user:password@127.0.0.1:5432/renop_db?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```
