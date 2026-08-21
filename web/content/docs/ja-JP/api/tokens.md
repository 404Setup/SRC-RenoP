---
title: トークン
order: 3
category: API
---

# ユーザーとアクセストークン

プレフィックス: `/api/tokens`

すべてのエンドポイントに **manager / admin** が必要です。一般ユーザーは自分のパスワードやアップロードトークンを
`/api/auth/profile/*` 経由で変更します。

ここでの「トークン」はアカウント記録です: ユーザー名、パスワードハッシュ、権限、任意のアップロードトークン。
`tokens.yaml` に永続化されます。

## `GET /api/tokens`

全アカウントを一覧。レスポンス: `application/x-protobuf`、`AccessTokenList`。

形状（JSON の例示）:

```json
{
  "tokens": [
    {
      "identifier": {"type": "PERSISTENT", "value": 1},
      "name": "admin",
      "created_at": "2026-01-01T00:00:00Z",
      "description": "…",
      "expires_at": null,
      "tokens": ["<upload-token-if-any>"],
      "permissions": ["manager", "canview:*", "canupdate:*"]
    }
  ]
}
```

パスワードハッシュは返りません。`tokens` 配列は存在する場合に平文のアップロードトークンを保持します。禁止 → 403。

## `GET /api/tokens/:name`

単一アカウントを **protobuf** `AccessTokenDto` (`application/x-protobuf`)
で。名前は大文字小文字を区別しません（小文字で保存）。不存在 → 404。

## `PUT /api/tokens/:name`

作成または更新。ボディ: `application/x-protobuf`、`CreateAccessTokenRequest`（JSON も受付可）。

| フィールド    | 意味                                                                               |
|---------------|------------------------------------------------------------------------------------|
| `is_create`   | `true` かつ名前が既に存在 → 409                                                    |
| `secret`      | 作成時に省略すると UUID パスワードを生成。更新時に省略するとパスワードは変更しない |
| `new_name`    | リネーム。先が競合 → 409                                                           |
| `permissions` | 指定時のみ権限リストを置換                                                         |

レスポンス: `application/x-protobuf`、`CreateAccessTokenResponse`

```protobuf
syntax = "proto3";

message CreateAccessTokenResponse {
  AccessTokenDto access_token = 1;
  string secret = 2; // 本リクエストで生成または指定された場合のみ存在
}
```

作成直後に `secret` を保存してください — 平文パスワードは後から復元できません。

## `DELETE /api/tokens/:name`

アカウント削除。`204`。不存在 → 404。

## ブラウザセッションと FIDO デバイス（マネージャー）

マネージャーは任意アカウントの **ブラウザログインセッション** と **FIDO セキュリティキーデバイス**
を一覧・取り消しできます。Basic/Bearer はセッションではありません。セッション秘密は返りません。

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf。アカウントなし → `404`。

### `POST /api/tokens/:name/sessions/revoke-all`

そのユーザーのブラウザセッションをすべて取り消し。マネージャーが **自分**
を対象にしたときはこのリクエストのセッションを残します。応答: `StatusOk` protobuf。

### `DELETE /api/tokens/:name/sessions/:session_id`

`public_id` で 1 セッションを取り消し。応答: `StatusOk` protobuf。存在しない id は no-op。

### `GET /api/auth/users/:username/fido`

マネージャーが指定ユーザーの登録済み FIDO デバイス一覧を取得します。応答: `FidoDeviceList` protobuf。

### `DELETE /api/auth/users/:username/fido/:device_id`

マネージャーが指定ユーザーの指定 FIDO デバイスを削除します。応答: `StatusOk` protobuf。

## `POST /api/tokens/:name/token`

管理者が別ユーザーのアップロードトークンを再発行（旧値を置換）。応答: `GenerateTokenResponse` protobuf (`token: "<uuid>"`).

`/api/auth/profile/token` と同じ考え方ですが、対象は他ユーザーです。
