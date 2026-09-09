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

manifest JSON の上限は 4 MiB です。local upload、mirror response、永続化済み Disk/S3 object に同じ上限を適用し、
超過した内容は parse や cache の前に拒否します。
永続化または配信の前に、宣言された SHA-256 digest と JSON bytes が正確に一致することも検証します。

---

## blob 操作

- **確認**: `HEAD /v2/:name/blobs/:digest`
- **ダウンロード**: `GET /v2/:name/blobs/:digest`
- **開始**: `POST /v2/:name/blobs/uploads/`（`?mount=<digest>&from=<other_repo>` 対応）
- **チャンク追加**: `PATCH /v2/:name/blobs/uploads/:uuid`
- **完了**: `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`

## リソースのロック

管理者とリポジトリのモデレーターは `PUT /api/docker/repositories/{repo}/locks?image={name}` で手動ロックを設定し、
`DELETE /api/docker/repositories/{repo}/locks?image={name}` で解除します。どちらも有効なブラウザーセッション Cookie
が必要で、API トークンでは管理できません。既存マニフェストの不変ダイジェストを指定し、`version` が空の場合はイメージ全体をロックします。

```json
{"version":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","mode":"read","reason":"trojan"}
```

モードは `write` と `read` です。公開理由は `hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、`squatting`、
`quality` です。読み取りロックは書き込みも禁止します。管理者、リポジトリのモデレーター、所有者、協力者（L0
を含む）のみがメタデータを確認でき、レイヤーや設定内容のダウンロードと当該イメージからのマウントは全員に禁止されます。カタログ、検索、タグ一覧、プロフィール、チームリソースにも同じ可視性ルールを適用します。

ダイジェストのロックはすべてのタグ別名と、マルチアーキテクチャインデックスが参照する子マニフェストおよび blob
に適用されます。記録した参照は再起動後も維持され、元のロックを解除するまで有効です。検査応答の `locks`、`inherited`、
`moderator`、`member`、`version_locked` は公開状態を示し、操作者の身元は公開しません。手動ロックを解除してもシステムや継承された制限は残ります。処理上限は
8,192 ダイジェストと 64 MiB のマニフェストメタデータです。不正または上限超過の参照グラフは `400` を返し、以前のロックを維持します。

禁止された変更は `423` と `X-Renop-Error-Code: resource_locked`
を返します。タグの再割り当て、公開承認、イメージロック中のチーム変更、ロックされたバージョンを含むイメージ全体の削除や廃止が対象です。共有
blob の削除や置換は、同じリポジトリ内の他のイメージのロックも確認します。凍結されたミラー内容は更新・置換されません。
