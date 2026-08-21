---
title: Токены
order: 3
category: API
---

# Пользователи и токены доступа

Префикс: `/api/tokens`

Каждый эндпоинт требует **manager / admin**. Обычные пользователи меняют свой пароль или upload-токен через
`/api/auth/profile/*`.

«Токен» здесь — запись учётной записи: имя, хеш пароля, права, опциональный upload-токен. Хранится в
`tokens.yaml`.

## `GET /api/tokens`

Список всех учётных записей. Ответ: `application/x-protobuf`, `AccessTokenList`.

Форма (JSON-иллюстрация):

```json
{
  "tokens": [
    {
      "identifier": {"type": "PERSISTENT", "value": 1},
      "name": "admin",
      "created_at": "2026-01-01T00:00:00Z",
      "description": "…",
      "expires_at": null,
      "tokens": ["<upload-token-if-any>"],
      "permissions": ["manager", "canview:*", "canupdate:*"]
    }
  ]
}
```

Хеши паролей никогда не возвращаются. Массив `tokens` содержит plaintext upload-токены, если они есть. Forbidden → 403.

## `GET /api/tokens/:name`

Одна учётная запись как **protobuf** `AccessTokenDto` (`application/x-protobuf`). Имена без учёта регистра (хранятся в
нижнем регистре). Нет → 404.

## `PUT /api/tokens/:name`

Создать или обновить. Тело: `application/x-protobuf`, `CreateAccessTokenRequest` (также JSON).

| Поле          | Смысл                                                                                |
|---------------|--------------------------------------------------------------------------------------|
| `is_create`   | `true` и имя уже есть → 409                                                          |
| `secret`      | При создании пропуск = сгенерировать UUID-пароль; при обновлении пропуск = не менять |
| `new_name`    | Переименование; конфликт → 409                                                       |
| `permissions` | Заменяет список прав только если передан                                             |

Ответ: `application/x-protobuf`, `CreateAccessTokenResponse`

```protobuf
syntax = "proto3";

message CreateAccessTokenResponse {
  AccessTokenDto access_token = 1;
  string secret = 2; // только когда сгенерирован или указан в этом запросе
}
```

Сохраните `secret` сразу после создания — plaintext-пароли потом не восстановить.

## `DELETE /api/tokens/:name`

Удалить учётную запись. `204`. Нет → 404.

## Браузерные сеансы и FIDO-устройства (менеджер)

Менеджеры могут просматривать и отзывать **браузерные сеансы входа** и **FIDO security key-устройства** любой учётной
записи. Basic/Bearer — не сеансы. Секреты сеансов не возвращаются.

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf. `404`, если учётной записи нет.

### `POST /api/tokens/:name/sessions/revoke-all`

Отозвать все браузерные сеансы пользователя. Если менеджер целится в **свою** учётную запись, сеанс этого запроса
сохраняется. Ответ: `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Отозвать один сеанс по `public_id`. Ответ: `StatusOk` protobuf. Отсутствующий id — no-op.

### `GET /api/auth/users/:username/fido`

Менеджер получает список зарегистрированных FIDO-устройств указанного пользователя. Ответ: `FidoDeviceList` protobuf.

### `DELETE /api/auth/users/:username/fido/:device_id`

Менеджер удаляет указанное FIDO-устройство указанного пользователя. Ответ: `StatusOk` protobuf.

## `POST /api/tokens/:name/token`

Админ перевыпускает upload-токен пользователя (заменяет старый). Ответ: `GenerateTokenResponse` protobuf
(`token: "<uuid>"`).

Та же идея, что `/api/auth/profile/token`, но для другого пользователя.
