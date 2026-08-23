---
title: Конфигурация базы данных
order: 3
category: Конфигурация
description: Подключение к SQLite, MySQL и PostgreSQL, пул соединений
---

# Конфигурация базы данных

```yaml
# SQLite
database:
  driver: "sqlite3"
  dsn: "renop.db"

# MySQL
database:
  driver: "mysql"
  dsn: "user:pass@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True"

# PostgreSQL
database:
  driver: "postgres"
  dsn: "postgres://user:pass@127.0.0.1:5432/renop_db?sslmode=disable"
```
