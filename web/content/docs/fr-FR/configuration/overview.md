---
title: Vue d'ensemble de la configuration
order: 1
category: Configuration
description: Paramètres de config.yaml, réseau et variables d'environnement
---

# Vue d'ensemble de la configuration

Fichier principal : `config.yaml`.

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
