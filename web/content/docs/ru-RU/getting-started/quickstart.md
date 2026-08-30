---
title: Быстрый старт
order: 3
category: Начало работы
description: Первый запуск, администратор, health check и создание репозиториев
---

# Быстрый старт

## Запуск сервера

При первом запуске RenoP создаёт super-administrator `admin` в базе. Задайте пароль явно:

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

Без переменной RenoP генерирует случайный пароль и один раз печатает его в stdout. Сохраните его и откройте
`http://localhost:3000`. По умолчанию bind — `0.0.0.0:3000`; в production используйте TLS или trusted reverse proxy.

## Репозитории по умолчанию и новые

Начальный `repositories.yaml` содержит три совместимых Maven-репозитория:

| Путь         | Видимость | Политика                     |
|:-------------|:----------|:-----------------------------|
| `/releases`  | `PUBLIC`  | Maven, redeployment запрещён |
| `/snapshots` | `PUBLIC`  | Maven, redeployment разрешён |
| `/private`   | `PRIVATE` | Maven, требуется вход        |

npm, Cargo, Docker и `files` создаются явно в управлении. Docker images и npm packages резервируются на странице
репозитория до push. Cargo names создаются после upstream check. Maven требует проверенный domain из меню аккаунта.

## Проверка здоровья

```bash
curl -s http://localhost:3000/api/status/health
# Output: "UP"
```

Метрики protobuf доступны в `/api/status/instance`. Health показывает только ответ process; до production traffic
проверьте базу и storage реальной аутентифицированной операцией.

## Важные переменные

| Переменная                     | По умолчанию          | Назначение                                 |
|:-------------------------------|:----------------------|:-------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`         | Путь основной конфигурации                 |
| `RENOP_REPOSITORIES`           | `repositories.yaml`   | Путь настройки репозиториев                |
| `RENOP_INDEX`                  | `index.json`          | Путь snapshot файлового индекса            |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | Генерируется один раз | Начальный пароль, если `admin` отсутствует |

Аккаунты, sessions, teams, API Token, audit и messages находятся в базе и не имеют YAML path variables.

## Следующие шаги

- [Обзор конфигурации](../configuration/overview.md) — TLS, база, proxy, previews и updater
- [Репозитории и зеркала](../configuration/repositories.md) — Engines, visibility, upstream, migration и S3
- [Maven и Gradle](../guides/maven-client.md) — Проверка domain и JVM clients
- [Cargo Registry](../guides/cargo-registry.md) — Создание репозитория и публикация crates
- [Docker Registry](../guides/docker-registry.md) — Создание image до push и настройка клиента
- [Реестр npm](../guides/npm-registry.md) — Резервирование пакетов и настройка совместимых клиентов
