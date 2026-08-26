---
title: API состояния и телеметрии
order: 9
category: Справочник API
description: Проверка здоровья, метрики, снимки и защищённая диагностика
---

# API состояния и телеметрии

Указанные ответы используют protobuf. Проверка здоровья и текущее состояние публичны; диагностика памяти требует
администратора и `server.debug_mode`, включённый при запуске процесса.

## Здоровье и hash интерфейса

- **Здоровье**: `GET /api/status/health` возвращает `"UP"`, пока процесс обслуживает запросы.
- **Hash**: `GET /api/status/hash` возвращает hash встроенных ресурсов для обнаружения необходимости перезагрузки UI.

## Текущее состояние экземпляра

- **Путь**: `GET /api/status/instance`
- **Формат**: protobuf `InstanceStatus`.
- **Содержимое**: версия, uptime, RSS/VSS, диск, goroutine, CPU, число ошибок, debug и состояние обновления.

### Декодированный пример

```json
{
  "version": "1.0.0",
  "uptime": 86400,
  "used_memory": 33554432,
  "vss_memory": 268435456,
  "renop_used_disk": 5242880000,
  "disk_used": 107374182400,
  "disk_total": 536870912000,
  "used_threads": 24,
  "logical_cores": 16,
  "failures_count": 0,
  "debug_mode": false
}
```

## Исторические снимки и диагностика

- **Снимки**: `GET /api/status/snapshots` возвращает `StatusSnapshotList` со временем, памятью, goroutine, открытыми
  файлами и VSS.
- **Heap profile**: `GET /api/debug/memory/heap` (`?gc=0` пропускает предварительный GC).
- **Allocation profile**: `GET /api/debug/memory/allocs`.
- **Goroutine profile**: `GET /api/debug/memory/goroutine`.
- **Runtime breakdown**: `GET /api/debug/memory/runtime` (`?gc=1` запускает GC).

```json
{
  "snapshots": [
    {
      "timestamp": 1787731200000,
      "used_memory": 33554432,
      "used_threads": 24,
      "open_files": 18,
      "vss_memory": 268435456
    }
  ]
}
```

Бинарные pprof открываются через `go tool pprof` или Speedscope. Если debug mode не был активен при запуске,
диагностические маршруты возвращают `403` даже администратору.
