---
title: Репозитории и зеркала
order: 2
category: Конфигурация
description: Настройка repositories.yaml, видимость, зеркалирование и S3
---

# Репозитории и зеркала

Файл конфигурации: `repositories.yaml`.

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC          # PUBLIC | HIDDEN | PRIVATE
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false
```
