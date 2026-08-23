---
title: Docker / OCI レジストリ API
order: 6
category: API リファレンス
description: OCI Distribution Spec v2 エンドポイント
---

# Docker / OCI レジストリ API

- `GET /v2/` - バージョン確認
- `GET /v2/token` - トークン取得
- `GET /v2/_catalog` - リポジトリ一覧
- `GET /v2/:name/tags/list` - タグ一覧
- `GET/PUT /v2/:name/manifests/:reference` - マニフェスト操作
- `GET/HEAD/POST/PUT /v2/:name/blobs/...` - Blob 操作
