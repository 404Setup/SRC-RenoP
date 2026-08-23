---
title: Реестр Docker и OCI
order: 3
category: Руководства
description: Пуш и пулл контейнеров с помощью Docker и Podman
---

# Реестр Docker и OCI

```bash
docker login localhost:3000
docker tag my-app:latest localhost:3000/my-org/my-app:1.0.0
docker push localhost:3000/my-org/my-app:1.0.0
docker pull localhost:3000/my-org/my-app:1.0.0
```
