---
title: API 概要
order: 1
category: API リファレンス
description: RenoP HTTP API 概要、プロトコル、ステータスコード
---

# RenoP HTTP API

デフォルトポート: `http://localhost:3000`

- `/api/*`: 管理 API
- `/{repo}/*`: Maven リポジトリ
- `/index/*`: Cargo インデックス
- `/v2/*`: Docker / OCI レジストリ

## 認証ヘッダー

- `Set-Cookie: renop_session=...`
- `Authorization: Bearer <token>`
- `Authorization: Basic <base64>`
