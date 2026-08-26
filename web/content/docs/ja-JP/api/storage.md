---
title: ストレージとアップロード API
order: 10
category: API リファレンス
description: リポジトリ直接操作と制限付き再開可能アップロード
---

# ストレージとアップロード API

直接ストレージルートは Maven と `files` 用です。Cargo と Docker はネイティブプロトコルを使用します。
変更操作では API Token scope、リポジトリ権限、形式、Maven ドメインポリシーをすべて確認します。

## 1. リポジトリ直接操作

正規パスは `/{repo}/{path...}` です。読み取りは HTTP validator と byte range に対応します。`HIDDEN` は
一覧に出ませんが正確なパスで読めます。`PRIVATE` は認可が必要です。

### ダウンロード

- **要求**: `GET /{repo}/{path}` または `HEAD /{repo}/{path}`
- ローカルにないファイルは有効ミラーから解決し、設定済みポリシーに従ってキャッシュできます。

### アップロード

- **要求**: `PUT /{repo}/{path}`
- **認証**: パスワード、または `repository:publish` を持つ API Token と現在の書き込み/ドメイン権限。
- Maven は検証済みドメイン下の有効な座標とメタデータのみ受理します。`files` は安全化した任意パスと
  上書きを許可します。

### 削除

- **要求**: `DELETE /{repo}/{path}`
- **認証**: `repository:delete` を持つ API Token または許可された資格情報と、現在の削除権限。

## 2. 分割再開可能アップロード

メタデータは protobuf、各 part は生バイナリです。サーバーが最終保存先を所有し、part サイズと session 数を
制限し、放棄された一時ファイルを削除します。

### 初期化

- **パス**: `POST /api/upload/chunked/`
- **Content-Type**: `ChunkedUploadInitRequest` の `application/x-protobuf`。
- `purpose` は `storage` または `updater`。storage の `path` はリポジトリ名から始めます。

```json
{
  "purpose": "storage",
  "filename": "app-1.0.0.jar",
  "size": 524288000,
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "generate_checksums": true,
  "chunk_size": 4194304,
  "gpg_signature_expected": false
}
```

### part のアップロード

- **パス**: `PUT /api/upload/chunked/{upload_id}/{index}`
- **Content-Type**: `application/octet-stream`。
- part は並列送信できます。受理済み index の再送は冪等で、長さが違う part は拒否されます。

### 完了または中止

- **完了**: `POST /api/upload/chunked/{upload_id}/complete`
- **中止**: `DELETE /api/upload/chunked/{upload_id}`
- 完了処理は 1 件だけ成功し、全 part と権限を再確認してリポジトリゲート経由で確定します。

```json
{
  "status": "created",
  "message": "",
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "release_id": ""
}
```

Maven で GPG が必須の場合、隔離中は `release_id` を含む `202 Accepted` になることがあります。
`purpose=updater` の成功はリポジトリパスではなく `ready_to_restart` を返します。
