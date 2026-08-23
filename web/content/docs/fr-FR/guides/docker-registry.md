---
title: Registre Docker et OCI
order: 3
category: Guides
description: Pousser et tirer des images avec Docker et Podman
---

# Registre Docker et OCI

```bash
docker login localhost:3000
docker tag my-app:latest localhost:3000/my-org/my-app:1.0.0
docker push localhost:3000/my-org/my-app:1.0.0
docker pull localhost:3000/my-org/my-app:1.0.0
```
