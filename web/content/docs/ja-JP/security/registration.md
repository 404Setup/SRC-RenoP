---
title: アカウント登録
order: 6
category: セキュリティ
description: 登録、メール確認、GitHub アカウント作成の設定
---

# アカウント登録

## 登録設定

登録は既定で無効です。管理者がサービス設定で有効にできます。無効にすると `/account/register` と登録操作は `404` を返しますが、公開状態 API は利用できます。トップレベル設定の既定値は次のとおりです。

```yaml
registration:
  enabled: false
  ip_limit: 1
  ip_interval: {value: 3, unit: week}
  provider_cooldown: {value: 12, unit: hour}
```

IP 枠を消費するのは登録成功時だけです。既定は三週間に一アカウントで、最初の登録成功から期間が始まります。同じアドレスを表す IPv4・IPv6 表記は同じ制限を共有します。退会しても枠は戻らず、制限は再起動後も維持されます。件数は 1–10,000、期間は正数で、単位は `minute`、`hour`、`day`、`week`、`month`、最大 365 日です。一か月は 30 日です。

## アカウントの作成

ログインページから登録ページを開きます。ユーザー名は ASCII 英数字とアンダースコアの 4–18 文字で、小文字で保存されます。任意のニックネームは Unicode 36 文字までです。パスワードは必須で UTF-8 の 6–72 バイトです。

メール送信が有効な場合、メールアドレスと八桁の確認コードが必要です。コードは十分間有効で、誤入力は五回までです。同じブラウザーと IP アドレスで確認してください。送信には既存の直列キュー、宛先ポリシー、割り当て、レート制限が適用され、ページに配送状態が表示されます。メールが無効ならアドレスは任意です。成功後は新しい資格情報でログインします。メールが有効なら登録成功通知もキューに入ります。

## GitHub で登録

ローカルアカウントと未連携の GitHub ログインは確認ページを開きます。RenoP は `read:user read:org user:email` を要求し、no-reply を除く確認済みの実際の連絡先アドレスを選びます。メールコードは不要です。認可は十分間の HttpOnly Cookie で開始ブラウザーに結び付けられ、コールバック状態は一回限り使用できます。

十分以内に確認し、パスワードを設定します。確認前に利用可能なアカウントは作成されません。期限切れ後は確認待ちの個人情報を削除し、同じ GitHub ID の再登録開始を設定された期間、既定では期限切れから十二時間制限します。ユーザー名、ニックネーム、アバターは任意で取り込めます。ローカルの名前制限が適用され、利用できないユーザー名は手動入力が必要です。任意情報がない場合やアバターがサイズ・割り当てを超える場合も登録できます。

## 登録 API

公開 JSON リクエストは `Content-Type: application/json` が必要で、本文は 4,096 バイトまでです。確認には非公開の HttpOnly 登録 Cookie も使用します。パスワード、コード、メール照会チケットをブラウザーストレージに保存しないでください。

- `GET /api/auth/registration/status`: 利用可否とメール要件を取得.
- `GET /api/auth/registration/pending`: 現在のブラウザーの確認待ち情報を取得.
- `POST /api/auth/registration/code`: コードをキューに追加し、`202` と非公開の配送確認情報を返す.
- `POST /api/auth/registration`: 登録を確認.
- `GET /api/settings/registration`, `PUT /api/settings/registration`: 登録設定の取得・更新（管理者）.

```json
{
  "username": "new_user",
  "nickname": "New User",
  "email": "user@example.com",
  "password": "a unique long password",
  "code": "12345678"
}
```

GitHub 登録の確認では `provider: "github"` を使い、確認済みメールを保持して `code` を省略します。`import_avatar` を `true` にするとアバターを取り込みます。成功は `201` と `username`、`avatar_imported` を返し、ログインセッションは作成しません。競合は `409`、無効・期限切れは `400`、IP 制限・プロバイダーの待機期間は `429` です。アカウント、メール、連携、IP 計数、確認の消費は一つのトランザクションで確定します。
