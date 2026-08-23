---
title: Docker & OCI レジストリ
order: 3
category: クライアントガイド
description: Docker および Podman によるコンテナイメージのプッシュとプル
---

# Docker & OCI レジストリ設定

## 1. ログイン

```bash
docker login localhost:3000
# Username: admin
# Password: <パスワードまたはトークン>
```

## 2. タグ付けとプッシュ

```bash
docker tag my-app:latest localhost:3000/my-org/my-app:1.0.0
docker push localhost:3000/my-org/my-app:1.0.0
```

## 3. プルと実行

```bash
docker pull localhost:3000/my-org/my-app:1.0.0
docker run -d -p 8080:8080 localhost:3000/my-org/my-app:1.0.0
```
