---
title: API хранилища и загрузки
order: 10
category: Справочник API
description: Прямые операции и ограниченные возобновляемые загрузки
---

# API хранилища и загрузки

Прямые маршруты предназначены для Maven и `files`. Cargo и Docker используют нативные протоколы. Каждое изменение
проверяет scope API Token, права репозитория, формат и политику домена Maven.

## Прямые операции с репозиторием

Канонический путь — `/{repo}/{path...}`. Чтение поддерживает HTTP validators и диапазоны байтов. `HIDDEN` не попадает в
списки, но читается по точному пути; `PRIVATE` требует авторизацию.

### Скачивание

- **Запрос**: `GET /{repo}/{path}` или `HEAD /{repo}/{path}`
- Отсутствующий локальный файл может быть получен с включённого зеркала и кеширован по настроенной политике.

### Загрузка

- **Запрос**: `PUT /{repo}/{path}`
- **Аутентификация**: пароль или API Token с `repository:publish` и текущие права записи/домена.
- Maven принимает только корректные координаты и метаданные проверенного домена. `files` принимает безопасные
  произвольные пути и разрешает замену.

### Удаление

- **Запрос**: `DELETE /{repo}/{path}`
- **Аутентификация**: API Token с `repository:delete` или другой разрешённый способ и текущее право удаления.

## Блочные возобновляемые загрузки

Метаданные передаются в protobuf, части — сырые бинарные данные. Сервер контролирует конечный путь, ограничивает размер
и число сессий и удаляет заброшенные временные файлы.

### Инициализация

- **Путь**: `POST /api/upload/chunked/`
- **Content-Type**: `application/x-protobuf` с `ChunkedUploadInitRequest`.
- `purpose` — `storage` или `updater`. Для storage поле `path` начинается с имени репозитория.

```json
{
  "purpose": "storage",
  "filename": "app-1.0.0.jar",
  "size": 524288000,
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "generate_checksums": true,
  "chunk_size": 4194304,
  "gpg_signature_expected": false
}
```

### Загрузка части

- **Путь**: `PUT /api/upload/chunked/{upload_id}/{index}`
- **Content-Type**: `application/octet-stream`.
- Части можно отправлять параллельно. Повтор принятого index идемпотентен; неверная длина отклоняется.

### Завершение или отмена

- **Завершить**: `POST /api/upload/chunked/{upload_id}/complete`
- **Отменить**: `DELETE /api/upload/chunked/{upload_id}`
- Завершение имеет одного победителя, повторно проверяет части и права и фиксирует данные через gate репозитория.

```json
{
  "status": "created",
  "message": "",
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "release_id": ""
}
```

При обязательном GPG возможен `202 Accepted` с `release_id` на время карантина. Для `purpose=updater` успех возвращает
`ready_to_restart`, а не путь репозитория.
