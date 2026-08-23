---
title: Dépôts et miroirs
order: 2
category: Configuration
description: Configuration de repositories.yaml, visibilité, miroirs et S3
---

# Dépôts et miroirs

Fichier : `repositories.yaml`.

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
