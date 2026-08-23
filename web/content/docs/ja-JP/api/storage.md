---
title: ストレージ & アップロード API
order: 10
category: API リファレンス
description: Maven ファイル操作および大容量ファイル分割アップロード
---

# ストレージ & アップロード API

- `GET/PUT/DELETE /{repo}/{path}` - 直接ファイル操作
- `POST /api/upload/chunked` - チャンクアップロード初期化
- `PUT /api/upload/chunked/:id?chunk_index=0` - チャンク送信
- `POST /api/upload/chunked/:id/complete` - アップロード完了・結合
