---
title: GPG 署名 API
order: 11
category: API リファレンス
description: GPG 公開鍵の登録および隔離アーティファクトの確認
---

# GPG 署名 API

- `GET /api/auth/profile/gpg` - 登録済み公開鍵一覧
- `POST /api/auth/profile/gpg` - 公開鍵の登録
- `GET /api/auth/profile/gpg/releases` - 隔離中の未検証リリース一覧
