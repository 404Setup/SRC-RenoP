---
title: Cargo レジストリ API
order: 5
category: API リファレンス
description: Sparse Index、crate の公開、ダウンロード、yank
---

# Cargo レジストリ API

RenoP は Cargo Registry と Sparse Index の仕様を実装します。

## Sparse Index 設定 (`config.json`)

- **パス**: `GET /{repo}/config.json` または `GET /{repo}/index/config.json`
- **用途**: Cargo が初回接続時に読み、レジストリの API を検出します。

### JSON レスポンス

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## Sparse Index メタデータ

- **パス**: `GET /{repo}/index/{prefix}/{crate_name}`
- **用途**: Cargo 標準の crate 名シャーディングに従った行区切り JSON を返します。

---

## crate の公開

- **パス**: `PUT /{repo}/api/v1/crates/new`
- **認証**: `Authorization: <token>` の Token が必要です。
- **本文**: 4 バイトの JSON 長、JSON メタデータ、`.crate` バイナリアーカイブの順です。
- **名前競合**: 正規化名がローカルまたは適用対象ミラーに存在する最初の公開は `409 Conflict` になります。
  上流確認が確定できない場合は `503 Service Unavailable` を返します。

---

## crate のダウンロード

- **パス**: `GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **レスポンス**: `application/x-tar` の `.crate` アーカイブです。

---

## yank と unyank

- **Yank**: `DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank**: `PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **認証**: crate 所有者または管理者です。
