---
title: Обзор API
order: 1
category: API
---

# RenoP HTTP API

Адрес прослушивания по умолчанию: `0.0.0.0:3000`.

| Путь        | Назначение                                                 |
|-------------|------------------------------------------------------------|
| `/api/*`    | Management API (вход, настройки, статус, …)                |
| `/{repo}/…` | Раскладка Maven-репозитория (скачивание/загрузка/удаление) |

Тела ошибок часто — простой текст (`Unauthorized`, `Forbidden`, `Not found`). Сначала доверяйте коду статуса.

## Оглавление

| Файл                                     | Содержание                                                       |
|------------------------------------------|------------------------------------------------------------------|
| [authentication.md](./authentication.md) | Вход, сессии, права                                              |
| [tokens.md](./tokens.md)                 | Управление учётными записями (manager)                           |
| [maven.md](./maven.md)                   | Обзор, версии, badge, генерация POM                              |
| [gpg.md](./gpg.md)                       | Ключи GPG, подписанные загрузки и проверка                       |
| [status.md](./status.md)                 | Health и runtime-статус                                          |
| [settings.md](./settings.md)             | Домены конфигурации, репозитории, пересборка индекса             |
| [updater.md](./updater.md)               | Онлайн/офлайн обновления                                         |
| [storage.md](./storage.md)               | GET/PUT/DELETE на путях репозитория, chunked-загрузка            |
| [rate-limit.md](./rate-limit.md)         | Лимиты по IP, бан после сбоев auth, лимит одновременных запросов |

Машиночитаемая схема: [openapi.yaml](/assets/openapi.yaml). Proto-определения: `proto/api/v1/api.proto` (сгенерированный
Go-код в `pb/`).

## JSON и Protobuf

Большинство эндпоинтов по-прежнему JSON. Эти используют `application/x-protobuf`:

| Эндпоинт                                     | Направление        |
|----------------------------------------------|--------------------|
| `POST /api/auth/login`                       | request + response |
| `GET /api/auth/me`                           | response           |
| `GET /api/tokens`                            | response           |
| `GET /api/status/instance`                   | response           |
| `GET /api/status/snapshots`                  | response           |
| `GET /api/updater/status`                    | response           |
| `POST /api/settings/index/rebuild`           | request            |
| `GET /api/settings/domains`                  | response           |
| `GET /api/settings/domain/:name`             | response           |
| `PUT /api/settings/domain/:name`             | request            |
| `GET /api/settings/maven/repositories`       | response           |
| `PUT /api/settings/maven/repositories/:name` | request            |
| `GET /api/maven/details…`                    | response           |
| `GET /api/maven/repo-details/:repo`          | response           |
| `GET /api/maven/signatures…`                 | response           |
| `GET /api/auth/profile/gpg`                  | response           |
| `POST /api/auth/profile/gpg`                 | request + response |
| `GET /api/auth/profile/gpg/releases`         | response           |
| `POST /api/upload/chunked/`                  | request + response |
| `POST /api/upload/chunked/:id/complete`      | response           |

Имена полей совпадают с proto (snake_case). Генерируйте клиентов через `protoc` или следуйте кодекам `protobufjs` во
фронтенде.

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

```bash
# После входа имя cookie — renop_session
curl -s -b 'renop_session=<session-id>' \
  -H 'Accept: application/x-protobuf' \
  http://localhost:3000/api/auth/me \
  -o me.bin
```

## Аутентификация

Поддерживаемые носители:

1. Cookie: `renop_session=<id>`
2. `Authorization: Session <id>`
3. `Authorization: Basic base64(user:password_or_upload_token)`
4. `Authorization: Bearer <user>:<secret>` или `Bearer <upload-token>`
5. Только GET/HEAD: `?token=<session-or-bearer>`

Сессии истекают примерно через **7 дней** простоя и обновляются при активности.

| Роль                 | Возможности                                                   |
|----------------------|---------------------------------------------------------------|
| Аноним               | Чтение PUBLIC-репозиториев; management API в основном 401/403 |
| Обычный пользователь | Доступ к репозиториям через `canview:` / `canupdate:`         |
| manager / admin      | Пользователи, настройки, updater и другие management API      |

Подробности: [authentication.md](./authentication.md).

## Коды статуса

| Код | Значение                                                    |
|-----|-------------------------------------------------------------|
| 200 | OK (тело может быть пустым или plain text)                  |
| 201 | Загрузка создана                                            |
| 204 | Успех, без тела                                             |
| 400 | Плохие параметры / тело                                     |
| 401 | Не аутентифицирован или неверные учётные данные             |
| 403 | Запрещено, истекло или IP забанен после повторных 401/403   |
| 404 | Отсутствует; приватные чтения могут отдавать 404 вместо 403 |
| 409 | Конфликт (имя занято, обновление уже идёт)                  |
| 429 | Анонимный IP превысил rate limit                            |
| 503 | Перегрузка (например, лимит одновременных запросов)         |
| 507 | Недостаточно места на диске                                 |

Лимиты и anomaly-правила: [rate-limit.md](./rate-limit.md).

Версия экземпляра: `version` в `GET /api/status/instance`. Отдельного поля версии API нет.
