---
title: 認証
order: 2
category: API
---

# 認証とセッション

プレフィックス: `/api/auth`

アカウントは `tokens.yaml` に保存されます（`RENOP_TOKENS` で上書き）。権限は文字列のリストです。

## 権限

| 値                    | 意味                                            |
|-----------------------|-------------------------------------------------|
| `admin` / `manager`   | 管理 API（コード上は同等）                      |
| `canview:*`           | 全リポジトリの読み取り                          |
| `canview:<repo>`      | 1 リポジトリの読み取り                          |
| `canupdate:*`         | 全リポジトリへの書き込み                        |
| `canupdate:<repo>`    | 1 リポジトリへの書き込み                        |
| `allview` / `proview` | PRIVATE（および類似の制限付き）可視性の読み取り |
| `showing`             | HIDDEN リポジトリルートの一覧                   |

リポジトリの可視性:

- **PUBLIC** — 匿名読み取り可
- **HIDDEN** — ファイルは読めるが、ルート一覧には追加ロールが必要
- **PRIVATE** — `canview` / `allview` / `proview`、当該リポジトリの書き込み権限、または manager

書き込み（成果物の PUT/POST/DELETE）には常に `canupdate` または manager が必要です。

## ログイン

### `POST /api/auth/login`

本文: `application/x-protobuf`、`LoginRequest`

| フィールド | 型     | 制約                       |
|------------|--------|----------------------------|
| `name`     | string | 1–128 文字                 |
| `secret`   | string | 1–72 バイト（bcrypt 上限） |

成功時: `SessionDetails`（protobuf）と Cookie:

- 名前: `renop_session`
- HttpOnly、SameSite=Lax
- HTTPS 時に `Secure`（`X-Forwarded-Proto: https` / Cloudflare visitor HTTPS を含む）
- Max-Age ≈ 7 日

| ステータス | 理由                             |
|------------|----------------------------------|
| 401        | ユーザー名またはパスワードが誤り |
| 403        | アカウント期限切れ               |
| 400        | 本文が読めない                   |

セッション id は `renop_session` Cookie にのみ設定されます。ログイン応答の `session_token` は空です。ブラウザは Cookie を使い、スクリプトは同じ id を `Authorization: Session …` で送れます。

## 現在のユーザー

### `GET /api/auth/me`

現在のセッションの `SessionDetails`（protobuf）を返します。未認証 → 401。

| フィールド      | 意味                                                                                                                                                                          |
|-----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_token`  | アカウント概要（name、created_at、permissions など）                                                                                                                          |
| `permissions[]` | 展開済みロール（manager は追加で `access-token:manager` を得る）                                                                                                              |
| `routes[]`      | canview/canupdate 由来のパス権限（`route:read` / `route:write`）。manager は `*` に `route:write` も得て、クライアントが manager を特別扱いせずに書き込みゲートを再現できる。 |
| `session_token` | リクエストが `Session` ヘッダーを使った場合に設定                                                                                                                             |

書き込み UI（ブラウザアップロード、削除ボタン）とストレージの PUT/POST/DELETE は同じ実効書き込み権限が必要です: `admin`/
`manager`、`canupdate:*`、または `canupdate:<repo>`。

現在のセッションと食い違う場合は Cookie を更新します。

## ログアウト

### `POST /api/auth/logout`

セッションを無効化し Cookie を消去します。`204 No Content`。セッションがなくても 204。

## プロフィール

以下はすべてログイン済みユーザーが必要です。

### `PUT /api/auth/profile/password`

JSON:

```json
{"new_password": "6–72 bytes"}
```

```json
{"status": "success"}
```

長さが不正 → 400。

### `POST /api/auth/profile/token`

アップロードトークンを再生成（ユーザーごとに 1 つ。旧値は置換）。

```json
{"token": "<uuid>"}
```

Maven / curl:

```bash
curl -u admin:UPLOAD_TOKEN -T my.jar \
  http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar
```

Basic の secret にはアカウントパスワードまたはアップロードトークンを使えます（アカウント設定による）。

### `GET /api/auth/profile/sessions`

現在のユーザーの **ブラウザログインセッション** を一覧します。Basic / Bearer はセッションを **作成せず**、ここには出ません。セッション秘密（Cookie 値）は **返されません**。

応答: `application/x-protobuf`、`SessionList`

| フィールド（`sessions[]`） | 意味 |
|----------------------------|------|
| `public_id` | 取り消し API 用の不透明 ID（Cookie 秘密ではない） |
| `username` | アカウント名 |
| `ip` | 最後に見えたクライアント IP |
| `user_agent` | ログイン時の端末 / User-Agent |
| `created_at` | 作成（Unix ms） |
| `last_active` | 最終アクティブ（Unix ms） |
| `expires_at` | アイドル期限: `last_active` + タイムアウト（通常 7 日、Unix ms） |
| `current` | このリクエストのセッションなら `true` |

### `POST /api/auth/profile/sessions/revoke-others`

現在のユーザーについて、**このリクエストのセッション以外** のブラウザセッションをすべて取り消します。応答: `StatusOk` protobuf（`status: success`）。

呼び出し元が Basic/Bearer（ブラウザセッションなし）の場合、そのユーザーのブラウザセッションはすべて取り消されます。

### `DELETE /api/auth/profile/sessions/:session_id`

`public_id` で **自分の** セッションを 1 つ削除。応答: `StatusOk` protobuf。存在しない id は no-op。現在のセッションを取り消すと Cookie がクリアされます。

## マネージャーのセッション管理

マネージャー（`admin` / `manager`）は `/api/tokens` 配下で **任意** アカウントのブラウザセッションを確認・取り消しできます。

### `GET /api/tokens/:name/sessions`

そのユーザーの `SessionList` protobuf。アカウントなし → `404`。非マネージャー → `403`。

### `POST /api/tokens/:name/sessions/revoke-all`

そのユーザーのブラウザセッションをすべて取り消し。マネージャーが **自分** を対象にした場合、このリクエストのセッションは残します。応答: `StatusOk` protobuf。

### `DELETE /api/tokens/:name/sessions/:session_id`

`public_id` でそのユーザーのセッションを 1 つ取り消し。応答: `StatusOk` protobuf。存在しない id は no-op。

## クライアントが資格情報を送る方法

| シナリオ                      | 方法                                                     |
|-------------------------------|----------------------------------------------------------|
| ブラウザ UI                   | Cookie（ログイン時に設定）                               |
| 管理 API を呼ぶスクリプト     | `Authorization: Session …` または Cookie                 |
| Maven deploy                  | Basic: `username` + パスワードまたはアップロードトークン |
| CI のプライベートダウンロード | Basic / Bearer。PUBLIC リポジトリは認証不要              |

`Bearer name:secret` は Basic と同様（パスワードハッシュまたはアップロードトークン）。  
`Bearer <upload-token>`（ユーザー名なし）はトークン索引からユーザーを解決します。
