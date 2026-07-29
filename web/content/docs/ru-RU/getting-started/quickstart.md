---
title: Быстрый старт
order: 3
category: Начало работы
description: Первый запуск, пароль администратора, URL репозиториев
---

# Быстрый старт

## Первый запуск

При первом запуске RenoP создаёт учётную запись `admin`. Задайте её пароль через переменную окружения до старта
процесса:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Если переменная не задана, генерируется случайный пароль и записывается в журнал сервера. После запуска откройте
`http://localhost:3000`.

Войдите как `admin`. Учётные записи с правами manager или admin могут управлять артефактами, пользователями,
репозиториями и настройками в веб-интерфейсе.

## Репозитории по умолчанию

| Путь                              | Назначение |
|-----------------------------------|------------|
| `http://localhost:3000/releases`  | Releases   |
| `http://localhost:3000/snapshots` | Snapshots  |
| `http://localhost:3000/private`   | Private    |

Укажите эти URL в Maven-элементах `<repositories>` или `<distributionManagement>`.
Примеры: [Maven-клиент](./maven-client.md).

## Проверка состояния

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## Переменные окружения

| Переменная                     | По умолчанию        | Назначение                                                      |
|--------------------------------|---------------------|-----------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Сервер, frontend, хранилище, updater                            |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Репозитории, зеркала, S3 на репозиторий                         |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Учётные записи и токены                                         |
| `RENOP_INDEX`                  | `index.json`        | Индекс артефактов                                               |
| `RENOP_SESSIONS`               | `sessions.bin`      | Сессии входа (protobuf; устаревший `sessions.json` мигрируется) |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | генерируется        | Пароль первой учётной записи admin                              |

Большинство параметров также можно изменить в интерфейсе управления. После изменения адреса прослушивания или TLS
требуется перезапуск процесса.

## Далее

1. [Конфигурация](../configuration/overview.md) — адрес, TLS, брендинг
2. [Репозитории и зеркала](../configuration/repositories.md)
3. [Maven-клиент](./maven-client.md)
4. [HTTP API](../api/README.md)
