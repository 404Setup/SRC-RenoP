---
title: Быстрый старт
order: 3
category: Начало работы
description: Первый запуск, пароль admin, URL репозиториев
---

# Быстрый старт

## Первый запуск

При первом запуске создаётся `admin`. Пароль — до старта:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Если не задано — случайный пароль в логе сервера. Дальше `http://localhost:3000`.

Вход: `admin`. Manager в web UI — артефакты, пользователи, репозитории, настройки.

## Репозитории по умолчанию

| Путь                              | Назначение |
|-----------------------------------|------------|
| `http://localhost:3000/releases`  | Releases   |
| `http://localhost:3000/snapshots` | Snapshots  |
| `http://localhost:3000/private`   | Private    |

В Maven: `<repositories>` / `<distributionManagement>`. Примеры: [Maven-клиент](./maven-client.md).

## Переменные окружения

| Переменная                     | По умолчанию        | Назначение                        |
|--------------------------------|---------------------|-----------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Сервер, UI, storage, updater      |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Репозитории, зеркала, S3 на repo  |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Аккаунты и токены                 |
| `RENOP_INDEX`                  | `index.json`        | Индекс артефактов                 |
| `RENOP_SESSIONS`               | `sessions.json`     | Сессии входа                      |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | генерируется        | Пароль первого admin              |

Многое правится и в UI. После смены listen/TLS — перезапуск.
