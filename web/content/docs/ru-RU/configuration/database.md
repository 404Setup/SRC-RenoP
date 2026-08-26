---
title: Настройка базы данных
order: 3
category: Конфигурация
description: Подключения SQLite, MySQL и PostgreSQL и пул соединений
---

# Настройка базы данных

RenoP хранит аккаунты, RBAC, API Token, сессии, аудит, команды и сообщения в базе данных. Настройте блок `database` в
`config.yaml`. Миграции выполняются автоматически при запуске.

## 1. SQLite (по умолчанию)

SQLite встроен и не требует внешнего сервиса:

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

- `dsn` может быть относительным или абсолютным путём к файлу.
- RenoP создаёт схему и включает WAL для параллельного доступа.

## 2. MySQL 8.0+

MySQL подходит для внешней управляемой базы:

```yaml
database:
  driver: "mysql"
  dsn: "renop_user:password@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### Требования MySQL

- Рекомендуется MySQL 8.0 или новее.
- Используйте `utf8mb4` и `utf8mb4_unicode_ci` либо `utf8mb4_0900_ai_ci`.
- Аккаунт должен иметь право создавать и изменять таблицы схемы RenoP.

## 3. PostgreSQL

PostgreSQL работает через драйвер `jackc/pgx/v5`:

```yaml
database:
  driver: "postgres"
  dsn: "postgres://renop_user:password@127.0.0.1:5432/renop_db?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### Форматы DSN

- **URI**: `postgres://username:password@host:port/dbname?sslmode=disable`
- **Ключ-значение**: `host=127.0.0.1 port=5432 user=renop_user password=password dbname=renop_db sslmode=disable`

В production включите TLS по требованиям поставщика базы вместо `sslmode=disable`.

## 4. Параметры пула

| Параметр                | По умолчанию | Описание                                  |
|:------------------------|:-------------|:------------------------------------------|
| `max_open_conns`        | `25`         | Максимум открытых соединений              |
| `max_idle_conns`        | `25`         | Максимум неактивных соединений            |
| `conn_max_lifetime_sec` | `300`        | Максимальный срок соединения в секундах   |

Соотносите пул с лимитом SQL-сервера. Избыточное значение расходует память и соединения без гарантии роста throughput.
