---
title: セキュリティ認証
order: 16
category: Security
description: CAPTCHA プロバイダーとブラウザーの保護対象を設定する
---

# セキュリティ認証

## プロバイダーと範囲

セキュリティ認証の設定で CAPTCHA を構成します。無効、reCAPTCHA v2 チェックボックス、v2 非表示認証、v3、Cloudflare Turnstile、hCaptcha、Friendly Captcha v2 に対応します。対応するサイトキーと秘密/API キーを使用してください。

パスワードログイン、登録、手動メール、グローバルチーム作成、公開ドメイン作成、パッケージ作成を個別に有効化できます。Passkey と外部ログインはパスワード設定の対象外です。登録メール送信は手動メール設定を優先し、無効なら登録設定を使います。登録完了は別の操作です。

認証はブラウザーの対話操作と匿名操作が対象です。検証済み API トークンとプロトコルのパスワードは除外し、User-Agent では判定しません。Maven のブラウザーアップロードと分割初期化は新規パッケージを確認します。既存パッケージと許可済み自動公開の権限は維持します。

秘密キーは返しません。プロバイダーとサイトキーが同じなら空欄で維持し、変更や無効化で古い秘密キーを削除します。v3 の最低スコアは標準で 0.5 です。サーバーとプロバイダーのドメイン設定に実際のホストを許可してください。Friendly Captcha はグローバルと EU に対応します。

reCAPTCHA と Turnstile はホスト名を検証します。hCaptcha のホスト名は統計用のため、想定するサイトキーを検証します。Friendly Captcha はサイトキーを検証し、提供された場合はオリジンも確認します。

```yaml
captcha:
  provider: turnstile
  site_key: YOUR_SITE_KEY
  secret_key: YOUR_SECRET_KEY
  min_score: 0.5
  friendly_region: global
  scopes:
    password_login: true
    registration: true
    manual_mail: true
    super_team_create: false
    domain_create: false
    package_create: false
```

## ブラウザー認証

Cookie 設定で任意の認証サービスを許可した場合だけ、独立フレーム内に外部コードを読み込みます。キャンセル、ページ移動、同意撤回でフレームを削除します。サーバー側の検証成功が必須で、エラー、低スコア、操作やホストの不一致は拒否します。

操作開始前に明示的な CAPTCHA 要求が返った場合だけ、ブラウザーは一度再試行します。ネットワークエラー、メール送信結果不明、その他のエラーでは自動再実行しません。

## API 契約

`GET /api/captcha` は公開設定と必須 HttpOnly nonce Cookie を返します。`POST /api/captcha/verify` は JSON の `scope`、`provider`、`site_key`、`response` を受け取ります。CAPTCHA 応答は最大 16 KiB、JSON 本文は最大 32 KiB です。成功時の一度限りの `proof` は 120 秒有効です。

同じブラウザー Cookie と `X-Renop-Captcha` で証明を送り、操作を再試行します。証明は範囲、セッション、現在の設定に結び付きます。未指定は `428` と `X-Renop-Error-Code: captcha_required`、`X-Renop-Captcha-Scope` を返し、無効または再利用は `400`、サービス障害は `503` です。

`GET /api/settings/captcha` と `PUT /api/settings/captcha` は設定管理権限を必要とする JSON API です。秘密キーを返しません。設定は即時反映され、秘密設定の変更で未使用の証明は無効になります。

[reCAPTCHA](https://developers.google.com/recaptcha/docs/verify) · [reCAPTCHA v3](https://developers.google.com/recaptcha/docs/v3) · [Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/) · [hCaptcha](https://docs.hcaptcha.com/) · [Friendly Captcha](https://developer.friendlycaptcha.com/docs/v2/getting-started/verify)
