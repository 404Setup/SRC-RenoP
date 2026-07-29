---
title: Аутентификация
order: 2
category: API
---

# Аутентификация и сессии

Префикс: `/api/auth`

Учётные записи хранятся в `tokens.yaml` (переопределение: `RENOP_TOKENS`). Права — список строк.

## Права

| Значение              | Смысл                                            |
|-----------------------|--------------------------------------------------|
| `admin` / `manager`   | Management API (в коде эквивалентны)             |
| `canview:*`           | Чтение всех репозиториев                         |
| `canview:<repo>`      | Чтение одного репозитория                        |
| `canupdate:*`         | Запись во все репозитории                        |
| `canupdate:<repo>`    | Запись в один репозиторий                        |
| `allview` / `proview` | Чтение PRIVATE (и схожей ограниченной) видимости |
| `showing`             | Список корней HIDDEN-репозиториев                |

Видимость репозитория:

- **PUBLIC** — анонимное чтение
- **HIDDEN** — файлы читаемы; список корня требует доп. ролей
- **PRIVATE** — нужны `canview` / `allview` / `proview`, права записи на репозиторий или manager

Запись (PUT/POST/DELETE артефактов) всегда требует `canupdate` или manager.

## Вход

### `POST /api/auth/login`

Тело: `application/x-protobuf`, `LoginRequest`

| Поле     | Тип    | Ограничения               |
|----------|--------|---------------------------|
| `name`   | string | 1–128 символов            |
| `secret` | string | 1–72 байта (лимит bcrypt) |

При успехе: `SessionDetails` (protobuf) и cookie:

- Имя: `renop_session`
- HttpOnly, SameSite=Lax
- `Secure` при HTTPS (включая `X-Forwarded-Proto: https` / Cloudflare visitor HTTPS)
- Max-Age ≈ 7 дней

| Статус | Причина                 |
|--------|-------------------------|
| 401    | Неверное имя или пароль |
| 403    | Учётная запись истекла  |
| 400    | Нечитаемое тело         |

Идентификатор сессии выставляется только в cookie `renop_session`. Поле `session_token` в ответе login пустое; браузеры используют cookie, скрипты могут передать тот же id как `Authorization: Session …`.

## Текущий пользователь

### `GET /api/auth/me`

Возвращает `SessionDetails` (protobuf) для текущей сессии. Без auth → 401.

| Поле            | Смысл                                                                                                                                                                             |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_token`  | Сводка учётной записи (name, created_at, permissions, …)                                                                                                                          |
| `permissions[]` | Развёрнутые роли (manager получает доп. `access-token:manager`)                                                                                                                   |
| `routes[]`      | Права путей из canview/canupdate (`route:read` / `route:write`). Manager также получает `route:write` на `*`, чтобы клиенты зеркалировали write-гейты без особого случая manager. |
| `session_token` | Устанавливается, если запрос использовал заголовок `Session`                                                                                                                      |

Write UI (панель загрузки в браузере, кнопки удаления) и storage PUT/POST/DELETE требуют одного и того же эффективного
права записи: `admin`/`manager`, `canupdate:*` или `canupdate:<repo>`.

Обновляет cookie, если оно расходится с текущей сессией.

## Выход

### `POST /api/auth/logout`

Инвалидирует сессию и очищает cookie. `204 No Content`. Также 204, если сессии не было.

## Профиль

Все эти эндпоинты требуют вошедшего пользователя.

### `PUT /api/auth/profile/password`

JSON:

```json
{"new_password": "6–72 bytes"}
```

```json
{"status": "success"}
```

Неверная длина → 400.

### `POST /api/auth/profile/token`

Перегенерировать upload-токен (один на пользователя; старое значение заменяется).

```json
{"token": "<uuid>"}
```

Maven / curl:

```bash
curl -u admin:UPLOAD_TOKEN -T my.jar \
  http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar
```

В качестве Basic secret можно использовать пароль учётной записи или upload-токен — в зависимости от настройки.

### `GET /api/auth/profile/sessions`

Список **браузерных сеансов входа** текущего пользователя. Basic и Bearer **не** создают сеансы и здесь не появляются. Секрет сеанса (значение cookie) **никогда** не возвращается.

Ответ: `application/x-protobuf`, `SessionList`

| Поле (`sessions[]`) | Смысл |
|---------------------|--------|
| `public_id` | Непрозрачный id для API отзыва (не секрет cookie) |
| `username` | Имя учётной записи |
| `ip` | Последний известный IP клиента |
| `user_agent` | Устройство / User-Agent при входе |
| `created_at` | Создание (Unix мс) |
| `last_active` | Последняя активность (Unix мс) |
| `expires_at` | Истечение по простою: `last_active` + таймаут (обычно 7 дней, Unix мс) |
| `current` | `true`, если это сеанс данного запроса |

### `POST /api/auth/profile/sessions/revoke-others`

Отзывает все браузерные сеансы текущего пользователя **кроме** сеанса этого запроса. Ответ: `StatusOk` protobuf (`status: success`).

Если вызывающий использует Basic/Bearer (нет браузерного сеанса), отзываются все его браузерные сеансы.

### `DELETE /api/auth/profile/sessions/:session_id`

Удалить **одну из своих** сессий по `public_id`. Ответ: `StatusOk` protobuf. Отсутствующий id — no-op. Отзыв текущего сеанса очищает cookie.

## Управление сеансами менеджером

Менеджеры (`admin` / `manager`) могут просматривать и отзывать браузерные сеансы **любой** учётной записи через `/api/tokens`.

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf для этого пользователя. `404`, если учётной записи нет. `403`, если вызывающий не менеджер.

### `POST /api/tokens/:name/sessions/revoke-all`

Отозвать все браузерные сеансы пользователя. Если менеджер целится в **свою** учётную запись, сеанс этого запроса сохраняется. Ответ: `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Отозвать один сеанс пользователя по `public_id`. Ответ: `StatusOk` protobuf. Отсутствующий id — no-op.

## Как клиенты передают учётные данные

| Сценарий                 | Подход                                      |
|--------------------------|---------------------------------------------|
| Браузерный UI            | Cookie (ставится при входе)                 |
| Скрипты к management API | `Authorization: Session …` или cookie       |
| Maven deploy             | Basic: `username` + пароль или upload-токен |
| CI-скачивания private    | Basic / Bearer; PUBLIC не требует auth      |

`Bearer name:secret` ведёт себя как Basic (хеш пароля или upload-токен).  
`Bearer <upload-token>` (без имени) ищет пользователя через индекс токенов.
