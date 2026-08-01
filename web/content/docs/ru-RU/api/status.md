---
title: Статус
order: 5
category: API
---

# Статус и health

Префикс: `/api/status`

Аутентификация не требуется.

## `GET /api/status/health`

```json
"UP"
```

Liveness-проба.

## `GET /api/status/hash`

Хеш содержимого frontend-ассетов как JSON-строка (cache busting).

## `GET /api/status/instance`

Ответ: `application/x-protobuf`, `InstanceStatus`.

| Поле                                                   | Смысл                                              |
|--------------------------------------------------------|----------------------------------------------------|
| `version`                                              | Версия бинарника                                   |
| `development`                                          | Флаг dev-сборки                                    |
| `uptime`                                               | Миллисекунды с запуска                             |
| `used_memory` / `total_memory`                         | Физическая память (байты)                          |
| `vss_memory`                                           | Виртуальная память (байты)                         |
| `renop_used_disk`                                      | Использование хранилища RenoP                      |
| `disk_used` / `disk_total`                             | Диск                                               |
| `used_threads` / `available_threads` / `total_threads` | Потоки / goroutine                                 |
| `architecture` / `os`                                  | GOARCH / GOOS                                      |
| `logical_cores` / `physical_cores`                     | CPU                                                |
| `failures_count`                                       | Счётчик runtime-сбоев                              |
| `update_state`                                         | Состояние updater — см. [updater.md](./updater.md) |
| `debug_mode`                                           | Включён ли режим отладки при старте                |

## `GET /api/status/snapshots`

Исторические сэмплы. Ответ: protobuf `StatusSnapshotList`.

| Поле           | Смысл             |
|----------------|-------------------|
| `timestamp`    | Unix-миллисекунды |
| `used_memory`  | Память            |
| `used_threads` | Число потоков     |
| `open_files`   | Открытые файлы    |

Пустой список, если данных нет (не 404).

## API анализа отладки (`/api/debug`)

Требуется разрешение **manager** и `server.debug_mode: true` в конфигурационном файле при старте. Возвращает 403, если
режим отладки отключён или прав недостаточно.

### `GET /api/debug/memory/heap`

Экспортирует профиль кучи Go runtime (формат pprof).

### `GET /api/debug/memory/allocs`

Экспортирует профиль выделений памяти (формат pprof).

### `GET /api/debug/memory/goroutine`

Экспортирует профиль стека Goroutine (формат pprof).

### `GET /api/debug/memory/runtime`

Возвращает детализацию памяти Go runtime (стек/вне кучи/RSS). Ответ: `application/x-protobuf`, `RuntimeMemoryBreakdown`.
