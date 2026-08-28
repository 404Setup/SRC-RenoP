---
title: Docker / OCI Registry v2 API
order: 6
category: API リファレンス
description: OCI Distribution v2 と Docker Registry v2 の API
---

# Docker / OCI Registry v2 API

RenoP は OCI Distribution Spec v2 と Docker Registry v2 を実装します。

コンテナイメージは明示的なリソースです。push 資格情報を要求する前に
`POST /api/docker/repositories/:repo/images` またはリポジトリページで作成してください。blob や manifest の
API が暗黙にイメージを作ることはありません。非公開イメージは未許可のカタログから除外され、manifest と
参照 blob の読み取りには L0-L4 メンバーまたは管理者が必要です。

正規化名がローカルまたは適用対象の有効ミラーに存在する場合、作成は `409 Conflict` になります。上流確認が
確定できない場合は名前を予約せず `503 Service Unavailable` を返します。

管理 API は可読本文と `X-Renop-Error-Code` を返し、UI は生の本文ではなくコードを翻訳します。OCI API は
仕様で定められた `errors` 構造を使用します。

イメージページにはパッケージ単位の Markdown README があります。L3/L4 メンバーまたは管理者は
`PUT /api/docker/repositories/{repo}/images?image={name}` で更新できます。JSON の `description` は 512 KiB に
制限され、共通の要素と URL の許可リストを通して描画されます。

## バージョン確認

- **パス**: `GET /v2/` または `HEAD /v2/`
- **レスポンス**:
    - `200 OK` と `Docker-Distribution-API-Version: registry/2.0`
    - 認証が必要な場合は `401 Unauthorized` と
      `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"`

---

## Bearer Token 認証

- **パス**: `GET /v2/token` または `GET /v2/auth`
- **用途**: Basic Auth を短期 Docker Token に交換します。API Token は pull に `repository:read`、push に
  `repository:publish`、削除に `repository:delete` が必要です。各操作の付与前に可視性と L0-L4 も独立して
  確認します。

---

## カタログとタグ

### イメージ一覧

- **パス**: `GET /v2/_catalog`
- **JSON**: `{"repositories": ["my-org/my-app"]}`

### タグ一覧

- **パス**: `GET /v2/:name/tags/list`
- **JSON**: `{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## manifest 操作

- **取得**: `GET /v2/:name/manifests/:reference`
- **公開**: `PUT /v2/:name/manifests/:reference`（作成済みイメージと L1 以上が必要）
- **削除**: `DELETE /v2/:name/manifests/:reference`

---

## blob 操作

- **確認**: `HEAD /v2/:name/blobs/:digest`
- **ダウンロード**: `GET /v2/:name/blobs/:digest`
- **開始**: `POST /v2/:name/blobs/uploads/`（`?mount=<digest>&from=<other_repo>` 対応）
- **チャンク追加**: `PATCH /v2/:name/blobs/uploads/:uuid`
- **完了**: `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
