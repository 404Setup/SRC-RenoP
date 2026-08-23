---
title: Configuration de la base de données
order: 3
category: Configuration
description: Connexions SQLite, MySQL, PostgreSQL et pool de connexions
---

# Configuration de la base de données

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
