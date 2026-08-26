---
title: 認証 API
order: 2
category: API リファレンス
description: ブラウザセッション、プロフィール、ログイン方式、復旧、セッション失効
---

# 認証 API

ブラウザ認証は HttpOnly Cookie `renop_session` を使用します。秘密値はプロフィールやセッション一覧に
返さず、ヘッダーや URL でも受理しません。非公開のセキュリティ API はブラウザセッション専用で、
パスワードや API Token では呼び出せません。

## パスワードまたはメールでログイン

- **パス**: `POST /api/auth/login`
- **認証**: なし。
- **本文**: protobuf `LoginRequest`。以下は JSON 名です。`name` はユーザー名または非公開メールです。

### リクエスト

```json
{
  "name": "admin",
  "secret": "your_password"
}
```

### セッション結果

成功時は `HttpOnly`、`SameSite=Lax`、HTTPS では `Secure` の `renop_session` を設定します。protobuf
`SessionDetails` は権限とルートを返しますが、`session_token` は空です。

## Passkey と GitHub ログイン

- **Passkey 開始**: `POST /api/auth/fido/login/begin`
- **Passkey 完了**: `POST /api/auth/fido/login/finish`
- **GitHub 開始**: `GET /api/auth/github/start`
- **GitHub callback**: `GET /api/auth/github/callback`
- **GitHub 利用可否**: `GET /api/auth/github/status`

GitHub は管理者が OAuth を設定した場合だけ表示されます。ユーザーと Organization の読み取りを要求し、
不変 Provider ID と Principal のスナップショットを保存しますが、OAuth Access Token は保存しません。

## 現在のアカウントと公開プロフィール

- **現在のセッション**: `GET /api/auth/me`
- **非公開プロフィール**: `GET /api/auth/profile`
- **ユーザー名または表示名の更新**: `PUT /api/auth/profile`
- **パスワード更新**: `PUT /api/auth/profile/password`
- **ログアウト**: `POST /api/auth/logout`
- **公開プロフィール**: `GET /api/users/:username/profile`
- **パッケージ所属**: `GET /api/users/:username/memberships?format=cargo|docker|maven`

公開ルートはユーザー名を使用し、不変 ID は内部に保ちます。`HIDDEN` の所属は除外し、非公開所属は許可された
閲覧者だけに返します。

## アカウントセキュリティ

これらのルートは現在のブラウザセッションを要求し、`Cache-Control: no-store` を返します。

### メールとパスワードログイン方針

- **状態取得**: `GET /api/auth/profile/security`
- **メール設定**: `PUT /api/auth/profile/email`
- **パスワードログイン切替**: `PUT /api/auth/profile/password-login`
- Passkey または GitHub が残る場合だけパスワードログインを無効化できます。有効化には設定済みパスワードが必要です。

### 復旧コード

- **生成**: `POST /api/auth/profile/recovery-codes`
- **パスワード再設定**: `POST /api/auth/recovery/password`
- 12 個のコードは一度だけ表示され、保存するのは Argon2id verifier だけです。異なる未使用コード 4 個を
  原子的に消費し、既存セッションを失効してパスワードログインを再有効化します。

```json
{
  "identifier": "admin@example.com",
  "codes": ["CODE-ONE", "CODE-TWO", "CODE-THREE", "CODE-FOUR"],
  "new_password": "new_secure_password"
}
```

## ログイン方式の管理

- **Passkey 一覧**: `GET /api/auth/profile/fido`
- **Passkey 登録**: `POST /api/auth/profile/fido/register/begin` の後に
  `POST /api/auth/profile/fido/register/finish`
- **Passkey 削除**: `DELETE /api/auth/profile/fido/:device_id`
- **GitHub ID 取得**: `GET /api/auth/profile/github`
- **GitHub 切断**: `DELETE /api/auth/profile/github`

最後の有効なログイン方式は削除または無効化できません。

## ブラウザセッション

- **一覧**: `GET /api/auth/profile/sessions`
- **1 件を失効**: `DELETE /api/auth/profile/sessions/:session_id`
- **他をすべて失効**: `POST /api/auth/profile/sessions/revoke-others`

一覧には公開 ID、ログイン方式、時刻、IP、User-Agent が含まれますが、Cookie の秘密値は含まれません。
