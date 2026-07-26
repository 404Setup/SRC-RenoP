---
title: Updater
order: 7
category: API
---

# Updater

Префикс: `/api/updater`

`GET /status` публичен; `check` / `install` / `upload` / `restart` требуют **manager**.

То же состояние есть в `GET /api/status/instance` как `update_state`.

Типичный поток:

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

Ответ: `application/x-protobuf`, `UpdateState` (см. `proto/api/v1/api.proto`).

| Поле                   | Смысл                                                           |
|------------------------|-----------------------------------------------------------------|
| `status`               | `idle`, `available`, `downloading`, `ready_to_restart`, `error` |
| `latest_version`       | Строка последней версии                                         |
| `download_url`         | URL загрузки пакета                                             |
| `progress`             | 0–100 во время загрузки                                         |
| `error_message`        | Задано, когда `status` = `error`                                |
| `size`                 | Размер пакета (байты)                                           |
| `estimated_disk_space` | Оценка нужного свободного места (байты)                         |
| `release_date`         | Строка даты релиза                                              |
| `release_notes`        | Текст release notes                                             |
| `commit_sha`           | Коммит исходников                                               |
| `is_release`           | Сборка канала release                                           |

## `POST /api/updater/check`

| Query     | По умолчанию | Смысл                   |
|-----------|--------------|-------------------------|
| `channel` | `release`    | `release` или `nightly` |

```json
{
  "has_update": true,
  "current_version": "…",
  "latest_version": "…",
  "download_url": "…",
  "channel": "release",
  "size": 12345678,
  "estimated_disk_space": 40000000,
  "release_date": "…",
  "release_notes": "…",
  "commit_sha": "…",
  "is_release": true
}
```

Сбой проверки → 500, `{ "error": "…" }`.

## `POST /api/updater/install`

Асинхронная загрузка и распаковка по текущему `download_url`. Если пусто — fallback на nightly URL по умолчанию.

| Статус | Причина                                                 |
|--------|---------------------------------------------------------|
| 507    | Недостаточно диска                                      |
| 409    | Установка уже идёт (`Installation already in progress`) |

Немедленный успешный ответ:

```json
{"status": "started"}
```

Прогресс — опрос `/status`. Финальное состояние: `ready_to_restart`.

## `POST /api/updater/upload`

Офлайн-обновление: multipart zip. Поле формы `file` или `package`; должно быть `.zip`.

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

Этот однозапросный multipart-путь остаётся по умолчанию для небольших пакетов и не-UI клиентов.

### Многочастная офлайн-загрузка — опционально

Большие zip из диалога офлайн-обновления Dashboard могут использовать параллельную chunked-загрузку через общий session
API (только manager). Пакеты меньше **8 MiB** по-прежнему идут через
`POST /api/updater/upload`. Init/complete — **`application/x-protobuf`**
(`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`); части — сырые октеты.

Размер части выбирается динамически от общего размера (см. multi-part в [storage.md](./storage.md)); используйте
`chunk_size` / `chunk_count` из ответа init.

1. `POST /api/upload/chunked/` с `purpose=updater`, `filename` (должен оканчиваться на `.zip`), `size`
2. Параллельные `PUT /api/upload/chunked/:id/:index` для каждой части (идемпотентно; повтор PUT принятой части OK)
3. `POST /api/upload/chunked/:id/complete` — извлекает бинарник и ставит `ready_to_restart`

Поля complete protobuf: `status=ready_to_restart`, `message=…`.

## `POST /api/updater/restart`

Заменить бинарник подготовленным обновлением и перезапустить.

Не готов → 400 (`No update ready to install`).

```json
{"status": "restarting"}
```

Соединение после этого обрывается — это ожидаемо.
