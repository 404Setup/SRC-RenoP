---
title: トークン & ユーザー API
order: 3
category: API リファレンス
description: きめ細かな API トークンのライフサイクルとユーザー管理 API
---

# トークン & ユーザー API

秘密値の管理には HttpOnly `renop_session` ブラウザー Cookie が必要です。

- `GET /api/auth/profile/api-tokens/scopes` — 現在のアカウントが割り当て可能な権限範囲。
- `GET /api/auth/profile/api-tokens` — 秘密値を含まないメタデータと上限 50 件。
- `POST /api/auth/profile/api-tokens` — 作成。`rnp_pat_...` の秘密値は一度だけ返されます。
- `DELETE /api/auth/profile/api-tokens/{token_id}` — 即時失効。

自動化では `Authorization: Bearer <token>` を使用します。Basic はパッケージプロトコルに限定されます。
ユーザー管理 API は `/api/tokens` にあり、`admin:users` 権限が必要です。
