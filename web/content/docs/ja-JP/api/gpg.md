---
title: GPG 暗号 API
order: 11
category: API リファレンス
description: OpenPGP 公開鍵の管理と署名検証状態
---

# GPG 暗号 API

## 1. アカウントの GPG 鍵一覧

- **パス**: `GET /api/auth/profile/gpg`
- **認証**: 必須です。

### JSON レスポンス

```json
{
  "keys": [
    {
      "key_id": "9B27346A83C1D0EE",
      "fingerprint": "A518767AE71A1C38BCE3178C9B27346A83C1D0EE",
      "user_id": "Developer <dev@example.com>",
      "created_at": 1740000000
    }
  ]
}
```

---

## 2. 公開鍵の登録

- **パス**: `POST /api/auth/profile/gpg`
- **認証**: 必須です。
- **JSON 本文**:
  ```json
  {
    "public_key_armored": "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n-----END PGP PUBLIC KEY BLOCK-----"
  }
  ```
- **レスポンス**: 解析済み鍵メタデータを含む `200 OK` です。

---

## 3. 隔離中の公開一覧

- **パス**: `GET /api/auth/profile/gpg/releases`
- **用途**: 分離署名、鍵検証、最終公開を待って `.renop.tmp.gpg` に隔離されている成果物を返します。
