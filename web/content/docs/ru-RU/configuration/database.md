---
title: Настройка базы данных
order: 3
category: Конфигурация
description: Подключения SQLite, MySQL и PostgreSQL и пул соединений
---

# Настройка базы данных

RenoP хранит аккаунты, RBAC, API Token, сессии, аудит, команды и сообщения в базе данных. Настройте блок `database` в
`config.yaml`. Миграции выполняются автоматически при запуске.

## SQLite (по умолчанию)

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

## MySQL 8.0+

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

## PostgreSQL

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

## ClickHouse 26.9+

RenoP подключается к самостоятельно управляемому ClickHouse через нативный API `clickhouse.Open` из
`clickhouse-go/v2` и не использует слой совместимости `database/sql`. Выделенная база должна быть создана заранее:

```yaml
database:
  driver: "clickhouse"
  dsn: "clickhouse://renop_user:password@127.0.0.1:9000/renop_db?compress=lz4"
  max_open_conns: 8
  max_idle_conns: 4
  conn_max_lifetime_sec: 600
```

Официальная сборка должна включать `EmbeddedRocksDB`. RenoP создаёт изменяемые key-value таблицы с материализованными
составными ключами без коллизий и преобразует обновления строк в синхронные нативные мутации. В ClickHouse 26.9 нет
многооператорных транзакций, поэтому RenoP сериализует записи и сохраняет построчные снимки в журнале; прерванная
транзакция откатывается при следующем запуске. ClickHouse Cloud не поддерживается этим режимом хранения. Матрица
проверена с ClickHouse 26.9.1 и `clickhouse-go/v2` 2.48.0.

## Параметры пула

| Параметр                | По умолчанию | Описание                                |
|:------------------------|:-------------|:----------------------------------------|
| `max_open_conns`        | `25`         | Максимум открытых соединений            |
| `max_idle_conns`        | `25`         | Максимум неактивных соединений          |
| `conn_max_lifetime_sec` | `300`        | Максимальный срок соединения в секундах |

Соотносите пул с лимитом SQL-сервера. Избыточное значение расходует память и соединения без гарантии роста throughput.
