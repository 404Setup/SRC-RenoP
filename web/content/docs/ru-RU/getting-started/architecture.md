---
title: Архитектура системы
order: 4
category: Начало работы
description: Модульные сервисы, авторизация, streaming storage и асинхронные задачи
---

# Архитектура системы

RenoP — один Go process с явными границами транспорта, пакетных протоколов, авторизации, persistence и фонового
обслуживания. Встроенный frontend вызывает те же ограниченные API, что и внешние клиенты.

## Границы модулей

```text
Browser and package clients
        |
HTTP routing, rate limits, authentication, API-token policy
        |
Maven | Cargo | Docker | Files | Management services
        |
Repository gate and publication workflows
        |
Disk or S3 storage          SQL database
        |                       |
File index and mirrors      Identity, teams, audit, messages
```

- `internal/api` и middleware владеют HTTP contracts, поиском, аномалиями и границами credentials.
- Format services владеют Maven domains/catalogs, Cargo Sparse Index, Docker Distribution v2 и doc viewers.
- Database layer предоставляет dialect-aware transactions для SQLite, MySQL и PostgreSQL.
- Disk/S3 потоково передаёт крупные тела, file index предоставляет ограниченный обход metadata.

## Pipelines запросов и задач

### Streaming и consistency

Uploads/downloads идут между клиентом и Disk/S3 потоком. Hash, Brotli/ZIP extraction, mirror cache и GPG используют
bounded readers и временные файлы. Striped repository gate исключает race смены storage/engine с uploads, deletes,
mirror commits и финальной публикацией.

### Аутентификация и авторизация

Browser session доступен только в cookie, Basic — только стандартным пакетным протоколам. Scopes и точные цели Bearer
API Token пересекаются с текущими repository permissions и L0-L4 membership при каждом запросе. Неизменяемый user ID
сохраняет владение при смене имени.

### Асинхронная работа

Process-wide non-reentrant scheduler объединяет snapshots, cleanup, index, download counters и update checks. Audit,
GPG, Token mutations и file watching остаются serial workers, где важен порядок. Durable results идут в message center,
временный прогресс — в UI state или Toast.
