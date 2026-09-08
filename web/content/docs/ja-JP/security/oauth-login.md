---
title: 外部サービスでログイン
order: 7
category: セキュリティ
description: Microsoft、Google、GitLab、Cloudflare、Stack Exchange、カスタム OAuth プロバイダーの設定
---

# 外部サービスでログイン

## プロバイダーの設定

管理者のサービス設定で「外部サービスでログイン」を開き、プロバイダーを選択してクライアントを追加します。認証情報と正確なコールバック URL を設定し、有効にして保存してください。各クライアントには小文字の一意な `id` が必要です。最大 32 文字で、先頭を英字にし、英字、数字、アンダースコア、ハイフンを使用できます。`github` は予約済みです。ID は既存の連携を識別するため、保存後は画面から変更できません。プロバイダーは最大 32 件、アカウントごとの連携も最大 32 件です。GitHub は専用の設定欄を引き続き使用します。

プロバイダーに Web アプリを登録し、コールバック `https://renop.example/api/auth/oauth/<provider-id>/callback` を完全一致で許可してください。OAuth エンドポイントとコールバックには HTTPS が必要です。開発用の HTTP ループバックアドレスは例外です。保存した設定は新しい認可に即時適用され、設定変更により進行中の認可は無効になります。

| プリセット | アプリの設定とオプション | メールの扱い |
|---|---|---|
| `microsoft` | [Microsoft ID プラットフォーム](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc)。`tenant` は `common`、`organizations`、`consumers`、または Entra テナントの UUID です。アプリが対応するアカウントの種類と一致させてください。既定のスコープは `openid profile email` です。 | [UserInfo](https://learn.microsoft.com/en-us/entra/identity-platform/userinfo) はメールの確認済み状態を保証しないため、RenoP のコードが必要です。 |
| `google` | [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect)。既定のスコープは `openid profile email` です。 | `email_verified` がブール値の `true` の場合のみ、確認済みアドレスとして扱います。 |
| `gitlab` | [GitLab OpenID Connect](https://docs.gitlab.com/integration/openid_connect_provider/)。`base_url` の既定値は `https://gitlab.com` で、セルフホストのインスタンスも指定できます。既定のスコープは `openid profile email` です。 | 確認済みの連絡先メールが返されなければ、コードが必要です。 |
| `cloudflare` | [OAuth クライアントを作成](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/)し、[統合を設定](https://developers.cloudflare.com/fundamentals/oauth/integrate-with-cloudflare/)します。`openid` スコープは安定した主体識別子を返します。非公開クライアントは Cloudflare アカウントのメンバーに限定され、公開クライアントには Cloudflare のドメイン確認が必要です。 | このプリセットは確認済みメールを提供しないため、RenoP のコードが必要です。 |
| `stackexchange` | [Stack Apps アプリ](https://stackapps.com/help/api-authentication)を登録し、クライアント認証情報と API キーを設定して `site` を選択します。既定値は `stackoverflow` です。RenoP は PKCE 付きの[認可コードフロー](https://api.stackexchange.com/docs/authentication)を使用し、ネットワーク全体の `account_id` で識別します。 | 連絡先メールは提供されないため、RenoP のコードが必要です。 |
| `custom` | 認可、トークン、ユーザー情報の URL と JSON フィールドパスを設定します。OpenID Connect では発行者と JWKS も設定します。 | 対応するメール確認フィールドがブール値の `true` の場合のみ信用します。それ以外はコードが必要です。 |

Microsoft は、テナントとアプリ登録の設定に応じて個人アカウントと Entra ID アカウントに対応します。Cloudflare と Stack Exchange に架空のメールフィールドを設定する必要はありません。これらのユーザーの登録を許可する場合は、先に[メール送信](../configuration/mail.md)を有効にしてください。

## 設定と認証情報

プロバイダーは `server.oauth_providers` に保存されます。次の例では、認証情報を置き換えるまでクライアントを無効にしています。

```yaml
server:
  oauth_providers:
    - id: microsoft
      type: microsoft
      name: Microsoft
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/microsoft/callback
      tenant: common
    - id: example
      type: custom
      name: Example Identity
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/example/callback
      authorize_url: https://identity.example/authorize
      token_url: https://identity.example/token
      userinfo_url: https://identity.example/userinfo
      scopes: ""
      token_auth: client_secret_post
      disable_pkce: false
      claims:
        subject: user.id
        username: user.username
        name: user.name
        email: user.email
        email_verified: user.email_verified
        avatar: user.picture
```

`name` は UTF-8 で最大 80 バイト、`client_id` は最大 512 バイト、シークレットと API キーは最大 4,096 バイトです。`scopes` は空白区切りの文字列で、最大 1,024 バイトです。エンドポイント URL は最大 2,048 バイトです。組み込みプリセットは公式エンドポイントとフィールド対応を設定します。その他の形式には `custom` を使用してください。

カスタムの `claims` は、`user.id` や `items.0.id` のようなドット区切りの JSON パスと数値の配列インデックスでスカラー値を選択します。パスは最大 128 文字です。`subject` にはプロバイダーの安定した識別子を使い、名前やメールをアカウント識別子として使用しないでください。数値の識別子は精度を維持します。任意のプロフィール項目は省略できます。文字列の `"true"` ではメールは確認済みになりません。

カスタムの `token_auth` には `client_secret_post`、`client_secret_basic`、`none` を指定できます。PKCE S256 は既定で有効です。`disable_pkce: true` は、クライアントシークレットを持つカスタムプロバイダーでのみ使用できます。OpenID Connect では `issuer` と `jwks_url` を両方設定し、`scopes` に `openid` を含めてください。RenoP は ID トークンを必須として検証します。OIDC を使わない OAuth クライアントでは、認証されたユーザー情報レスポンスを使用します。

設定の読み取りでは `client_secret_configured` と `api_key_configured` が返され、シークレットの値は空です。空欄の書き込みで保存済みの値を保持できるのは、ID、プロバイダーの種類、クライアント ID、トークンエンドポイントが同じ場合のみです。`clear_client_secret` と `clear_api_key` で明示的に削除できます。クライアントやエンドポイントを変更した場合は、認証情報を再入力してください。プロバイダーを削除すると新しい認可は停止しますが、ユーザーが解除できるよう既存のアカウント連携は保持されます。

## 登録とアカウント操作

未連携のプロバイダーでログインすると、[アカウント登録](./registration.md)が始まります。ユーザーが確認してパスワードを設定するまで、アカウントやログインセッションは作成されません。どのプロバイダーにも、10 分の確認期限、プロバイダー登録の待機期間、IP ごとの上限、宛先ポリシー、ローカルのユーザー名・ニックネームの制限、アバターの容量制限が適用されます。メールが一致しても、既存アカウントが自動で選ばれたり統合されたりすることはありません。

確認待ちのレスポンスには `provider`、`provider_name`、`email_required`、`mail_enabled`、`avatar_available` が含まれます。`email_required` が true の場合、プロバイダーが未確認の候補アドレスを返していても、ユーザーはアドレスを入力して確認する必要があります。登録コードのエンドポイントには `provider` と `email` を一緒に送信してください。最終確認でも同じプロバイダー ID と受信した `code` を送信します。コードの再発行では元の確認期限や失敗回数はリセットされません。メールを利用できない場合、確認が必須のプロバイダーでは登録を完了できません。

プロバイダーが確認済みアドレスを返した場合、そのアドレスは確認中に変更できず、コードは不要です。ユーザー名、ニックネーム、アバターの取り込みは任意です。画像の欠落や容量超過によって登録が取り消されることはありません。名前にはインスタンスの制限が適用され、重複するユーザー名は手動で変更する必要があります。

プロフィール編集画面では、プロバイダーの連携や更新、画像の取り込み、最新の確認済みメールの使用、連携解除ができます。メール確認には新しい認可が必要で、ログイン連携は変更されません。メール確認フィールドのないプロバイダーには、この操作は表示されません。画像を取り込むには、選択した外部 ID が現在のアカウントに連携されている必要があります。サーバーは解除前に別の主要なログイン方法が残ることを確認します。[二段階認証](./two-step-verification.md)、アカウント停止、永久閉鎖は外部ログインにも適用されます。

## API

| メソッド | パス | 契約 |
|---|---|---|
| GET | `/api/auth/oauth/providers` | 設定済みプロバイダーの ID と名前の公開一覧 |
| GET | `/api/auth/oauth/:provider/start` | `intent=login`、`register`、`link`、`email`、`avatar`。ローカルの `return_to` は任意 |
| GET | `/api/auth/oauth/:provider/callback` | 一回限りの認可コードとブラウザーに紐づいた状態 |
| GET | `/api/auth/profile/oauth` | 非公開の連携状態、表示識別子、認可日時、許可された操作 |
| DELETE | `/api/auth/profile/oauth/:provider` | 解除時は `204`。最後のログイン方法の場合は `409` と `oauth_last_login_method` |
| GET | `/api/settings/oauth-providers` | 管理者用の `providers` と `presets`。シークレットは含まない |
| PUT | `/api/settings/oauth-providers` | 管理者用 JSON `{providers:[...]}` で一覧を置換。本文の上限は 128 KiB |

プロフィール操作には現在のブラウザーセッションが必要です。コールバックは `oauth` に安定した結果マーカー、`provider` に ID を返し、SPA が翻訳してパラメーターを削除します。エラー時にプロバイダーの生のレスポンスは表示しません。セッションのログイン方法は `oauth:<provider-id>` で、必要に応じて `+totp` または `+passkey` が付きます。

## GitLab の Maven 認可

過去 1 時間以内の GitLab.com 認可により、新規の `io.gitlab.<namespace>` ドメインを自動確認できます。RenoP が受け入れるのは、本人の名前空間と GitLab のグループ所有者クレームにあるトップレベルのグループです。公開グループへの参加、サブグループの所有、自前の GitLab インスタンスの ID では、親や GitLab.com の名前空間を認可できません。連携を更新すると所有権の証明が更新されます。ID ごとに最大 1,001 件の名前空間を保持します。公開プロフィールやグループの説明文による従来の確認も利用できます。

## セキュリティと運用

OAuth コールバックは、有効期間 10 分の HttpOnly Cookie、一回限りのサーバー状態、PKCE を使用します。外部認可の状態は最大 2,048 件保持します。設定が変わると、確認待ちのコールバックと登録は無効になります。OIDC は署名、発行者、対象、主体、nonce、有効期間、および提供されている場合のトークンハッシュを検証します。対応する署名は RS256 と ES256 です。プロバイダーのレスポンスは 1 MiB に制限され、リクエストにはタイムアウトがあり、設定済みの外向きプロキシを使用します。

RenoP は、プロバイダーの種類、クライアント、エンドポイント、検証済みの発行者から決まる認証元に、安定した主体識別子を紐づけて保存します。認証元を変更しても、既存の連携が別の ID サービスに移ることはありません。ログイン用のアクセストークンは保持しません。保護された画像に必要なトークンは、登録確認が完了または期限切れになるまで、確認待ち登録の中だけに暗号化して保持できます。この一時値は非公開の `mfa_encryption_key` で保護され、ブラウザーには返されません。

アカウントの閉鎖は、すべての外部連携を一つのトランザクションで解放します。解放された ID は別の有効なアカウントに連携できますが、閉鎖されたユーザー名は永久予約、メールは 14 日間保持、操作履歴は 30 日間保持のままです。遅れて到着したコールバックや更新で、閉鎖済みアカウントの連携を再作成することはできません。
