---
title: Обзор конфигурации
order: 1
category: Конфигурация
description: Параметры config.yaml, настройки сети и переменные окружения
---

# Обзор конфигурации

Основной конфигурационный файл: `config.yaml`.

```yaml
storage_path: "storage"
enable_javadoc_preview: true

server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  domains:
    - "localhost"
  max_active_requests: 512

database:
  enabled: true
  driver: "sqlite3"
  dsn: "renop.db"

proxy:
  selected: ""
  proxies: {}
```
