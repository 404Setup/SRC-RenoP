---
title: Updater
order: 7
category: API
---

# Updater

Префикс: `/api/updater`

`GET /status` публичен; `check` / `install` / `upload` / `restart` требуют **manager**.

То же состояние в `GET /api/status/instance` как `update_state`.

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

Ответ: `application/x-protobuf`, `UpdateState` (`proto/api/v1/api.proto`).

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

| Query     | По умолчанию                | Смысл                   |
|-----------|-----------------------------|-------------------------|
| `channel` | настройка `updater.channel` | `release` или `nightly` |

Пусто / неверно → `updater.channel` (по умолчанию `release`).

| Канал     | `info.json`                                           |
|-----------|-------------------------------------------------------|
| `nightly` | `https://mvnc.pkg.one/update/renop/nightly/info.json` |
| `release` | `https://mvnc.pkg.one/update/renop/stable/info.json`  |

Пакеты: `…/{nightly\|stable}/{version}/{file}`.

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

Ошибка → 500, `{ "error": "…" }`.

## `POST /api/updater/install`

Асинхронная загрузка/распаковка по текущему `download_url`.

| Статус | Причина                                                 |
|--------|---------------------------------------------------------|
| 507    | Недостаточно диска                                      |
| 409    | Установка уже идёт (`Installation already in progress`) |

```json
{"status": "started"}
```

Опрос `/status`. Готово: `ready_to_restart`.

## `POST /api/updater/upload`

Офлайн-обновление: multipart zip (`file` или `package`). Только `.zip`.

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

### Многочастная загрузка (опционально)

Большие zip — chunked upload (manager). Меньше **8 MiB** → один `POST /api/updater/upload`.

Init/complete: **`application/x-protobuf`** (`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`). Части —
сырые байты.

Размер части: [storage.md](./storage.md). Берите `chunk_size` / `chunk_count` из init.

1. `POST /api/upload/chunked/` — `purpose=updater`, `filename` (`.zip`), `size`
2. `PUT /api/upload/chunked/:id/:index` (параллельно, идемпотентно)
3. `POST /api/upload/chunked/:id/complete` → `ready_to_restart`

## `POST /api/updater/restart`

Если есть подготовленный бинарник обновления — применяет его и перезапускает процесс. Иначе перезапускает текущий
процесс без применения обновления.

```json
{"status": "restarting"}
```
