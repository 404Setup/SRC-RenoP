---
title: 外部サービスでログイン
order: 7
category: セキュリティ
description: Microsoft、Google、GitLab、Cloudflare、Stack Exchange、カスタム OAuth プロバイダーの設定
---

# 外部サービスでログイン

## プロバイダーの設定

管理者設定の **外部ログイン**で GitHub と他のプロバイダーをまとめて設定します。GitHub は固定 ID `github`
の組み込み項目で、有効化または無効化できます。他に最大 32 クライアントを追加できます。ID は小文字で始まり、英小文字、数字、アンダースコア、ハイフンを使う最大
32 文字の一意な値です。保存済み ID は既存の連携を識別するため変更できません。各アカウントは GitHub 連携を 1 件、その他の連携を最大
32 件保持できます。

互換性のため GitHub は `server.github_oauth` とコールバック `/api/auth/github/callback`
を引き続き使用します。既存の設定と連携は統合画面に自動表示されます。クライアント ID は最大 128 バイト、シークレットは最大
512 バイトです。空欄のシークレットはクライアント ID が同じ場合のみ保持されます。GitHub
のユーザー・組織認可、確認済みメール、手動アバター同期は引き続き利用できます。

以下のプロバイダー設定とプロトコルの詳細は追加の OAuth クライアントを説明します。GitHub は既存の認可フローと互換コールバックを保持します。

プロバイダーに Web アプリを登録し、コールバック `https://renop.example/api/auth/oauth/<provider-id>/callback`
を完全一致で許可してください。OAuth エンドポイントとコールバックには HTTPS が必要です。開発用の HTTP
ループバックアドレスは例外です。保存した設定は新しい認可に即時適用され、設定変更により進行中の認可は無効になります。

| プリセット      | アプリの設定とオプション                                                                                                                                                                                                                                                                                                                                                                                              | メールの扱い                                                                                                                                      |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `microsoft`     | [Microsoft ID プラットフォーム](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc)。`tenant` は `common`、`organizations`、`consumers`、または Entra テナントの UUID です。アプリが対応するアカウントの種類と一致させてください。既定のスコープは `openid profile email` です。                                                                                                             | [UserInfo](https://learn.microsoft.com/en-us/entra/identity-platform/userinfo) はメールの確認済み状態を保証しないため、RenoP のコードが必要です。 |
| `google`        | [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect)。既定のスコープは `openid profile email` です。                                                                                                                                                                                                                                                                         | `email_verified` がブール値の `true` の場合のみ、確認済みアドレスとして扱います。                                                                 |
| `gitlab`        | [GitLab OpenID Connect](https://docs.gitlab.com/integration/openid_connect_provider/)。`base_url` の既定値は `https://gitlab.com` で、セルフホストのインスタンスも指定できます。既定のスコープは `openid profile email` です。                                                                                                                                                                                        | 確認済みの連絡先メールが返されなければ、コードが必要です。                                                                                        |
| `cloudflare`    | [OAuth クライアントを作成](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/)し、[統合を設定](https://developers.cloudflare.com/fundamentals/oauth/integrate-with-cloudflare/)します。`/oauth2/userinfo` エンドポイントが安定した主体識別子（`sub`）を返します。非公開クライアントは Cloudflare アカウントのメンバーに限定され、公開クライアントには Cloudflare のドメイン確認が必要です。 | `user-details.read` が付与されている場合は確認済みメールを自動取得します。それ以外は RenoP のコードが必要です。                                   |
| `stackexchange` | [Stack Apps アプリ](https://stackapps.com/help/api-authentication)を登録し、クライアント認証情報と API キーを設定して `site` を選択します。既定値は `stackoverflow` です。RenoP は PKCE 付きの[認可コードフロー](https://api.stackexchange.com/docs/authentication)を使用し、ネットワーク全体の `account_id` で識別します。                                                                                           | 連絡先メールは提供されないため、RenoP のコードが必要です。                                                                                        |
| `custom`        | 認可、トークン、ユーザー情報の URL と JSON フィールドパスを設定します。OpenID Connect では発行者と JWKS も設定します。                                                                                                                                                                                                                                                                                                | 対応するメール確認フィールドがブール値の `true` の場合のみ信用します。それ以外はコードが必要です。                                                |

Microsoft は、テナントとアプリ登録の設定に応じて個人アカウントと Entra ID アカウントに対応します。Cloudflare と Stack
Exchange
に架空のメールフィールドを設定する必要はありません。これらのユーザーの登録を許可する場合は、先に[メール送信](../configuration/mail.md)
を有効にしてください。

## プロバイダー設定手順

### GitHub

GitHub の **Settings** -> **Developer settings** -> **OAuth Apps** を開き、 **New OAuth App** を選択します（組織の
Developer settings でも同様です）。

アプリケーション情報を入力します：

- **Application name**：アプリ名（例：`RenoP`）。
- **Homepage URL**：RenoP インスタンスのルート URL（例：`https://renop.example`）。
- **Authorization callback URL**：GitHub 専用のコールバック URL `https://renop.example/api/auth/github/callback`（通常の
  OAuth のサブパスではなく固定パスを使用します）。

登録後、 **Client Secret** を生成してコピーし、 **Client ID** も控えます。RenoP の管理者設定で組み込みの **GitHub**
を開き、認証情報を入力して有効化します。

### Microsoft Entra ID

Microsoft Entra 管理センターまたは Azure ポータルにサインインし、 **ID** -> **アプリケーション** -> **アプリの登録** から
**新規登録** を選択します。

登録内容を設定します：

- **名前**：アプリ名（例：`RenoP`）。
- **サポートされているアカウントの種類**：
    - 個人の Microsoft アカウントと組織アカウントの両方を許可する場合：「任意の組織ディレクトリ内のアカウントと個人の
      Microsoft アカウント」を選択し、RenoP で `tenant: common` を設定します。
    - 組織の職場・学校アカウントのみを許可する場合：「任意の組織ディレクトリ内のアカウント」を選択し、RenoP で
      `tenant: organizations` を設定します。
    - 個人アカウントのみを許可する場合：「個人の Microsoft アカウントのみ」を選択し、RenoP で `tenant: consumers` を設定します。
    - 自組織のみに制限する場合：「この組織ディレクトリ内のアカウントのみ」を選択し、RenoP で `tenant` にディレクトリ（テナント）ID
      を指定します。
- **リダイレクト URI**：プラットフォームに「Web」を選択し、`https://renop.example/api/auth/oauth/microsoft/callback` を入力します。

**証明書とシークレット** で新しいクライアントシークレットを作成し、 **値**（Value）をコピーします。 **API のアクセス許可**
で委任された `User.Read` と `openid`、`profile`、`email` を確認します。RenoP で種類 `microsoft` のプロバイダーを追加し、Client
ID、Client Secret、Tenant を設定します。

### Google Cloud

Google Cloud コンソールを開き、 **API とサービス** -> **認証情報** に移動します。

**OAuth 同意画面** を未設定の場合は先に設定します：

- ユーザータイプに **外部**（または組織内）を選択し、アプリ情報とスコープ `openid`、`.../auth/userinfo.email`、
  `.../auth/userinfo.profile` を追加します。

**認証情報** 画面で **認証情報を作成** -> **OAuth クライアント ID** をクリックします：

- **アプリケーションの種類**：ウェブ アプリケーション。
- **承認済みのリダイレクト URI**：`https://renop.example/api/auth/oauth/google/callback` を追加します。

作成後、 **クライアント ID** と **クライアント シークレット** をコピーします。RenoP で種類 `google` を追加し、Client ID と
Client Secret を入力します（既定のスコープは `openid profile email`）。

### GitLab

GitLab.com またはセルフホストの GitLab インスタンスでアプリケーションを登録します：

- インスタンス全体： **Admin Area** -> **Applications**。
- ユーザー設定： **User Settings** -> **Applications**。
- グループ設定：グループの **Settings** -> **Applications**。

設定項目：

- **Name**：アプリ名。
- **Redirect URI**：`https://renop.example/api/auth/oauth/gitlab/callback`。
- **Confidential**：有効のままにします。
- **Scopes**：`openid`、`profile`、`email`、`read_user` にチェックを入れます。

保存後、 **Application ID**（Client ID）と **Secret** を控えます。RenoP で種類 `gitlab` のプロバイダーを追加し、セルフホストの場合は
`base_url`（例：`https://gitlab.example.com`）も設定します。

### Cloudflare

Cloudflare ダッシュボードで **アカウントを管理** -> **OAuth クライアント**（Manage Account -> OAuth clients）を開きます。

**クライアントを作成**（Create client）をクリックし、アプリケーションとプロトコルの設定を入力します：

- **Client name**：クライアント名を入力します（例：`RenoP`）。
- **Response type**：Token、ID Token、Code をサポートしています（複数選択可）。 **必ず `Code` を選択してください**（RenoP
  は認可コードフローを使用します）。
- **Grant type**：Authorization Code と Refresh Token をサポートしています（複数選択可）。 **必ず `Authorization Code`
  を選択してください**。
- **Redirect URLs**：RenoP の完全なコールバック URL
  を入力します（例：`https://renop.example/api/auth/oauth/cloudflare/callback`。プロバイダー ID を変更した場合は該当箇所を置き換えてください）。
- **Client URL**（任意）：RenoP インスタンスのホームページ（例：`https://renop.example`）。後で公開（Public）クライアントに昇格させる場合（DNS
  TXT レコードによるドメイン確認が必要）は必須です。Cloudflare アカウントのメンバーのみが利用する非公開（Private）クライアントの場合は任意です。
- **Token authentication method**（3 つの中から 1 つ選択。RenoP 側の設定と一致させる必要があります）：
    - **Client Secret POST**（既定かつ推奨）：クライアントがトークン交換リクエスト本文で Client Secret を送信します。RenoP
      ではトークン認証方式の既定値が `client_secret_post` です。
    - **Client Secret Basic**：クライアントが HTTP Basic Authorization ヘッダー経由で Client Secret を送信します。Cloudflare
      側で選択した場合は、RenoP 側でも `client_secret_basic` を指定してください。RenoP は HTTP 401 応答時に自動再試行・交渉も行います。
    - **PKCE**：クライアントシークレット不要の公開クライアントモードです。RenoP でトークン認証方式を `none`
      に設定します（クライアントシークレットは不要です）。
- **Post-logout redirect URLs**（任意）：ログアウト後のリダイレクト先（例：`https://renop.example`）。
- **Allowed CORS origins**（任意）：クロスオリジンアクセスを許可するオリジン。トークン交換は RenoP
  サーバー側で行われるため、通常は空のままで構いません。

**Continue** をクリックして権限スコープ（Scopes）を設定します：

- クライアントに必要な API 権限を選択します（`User Details: Read` など少なくとも 1 つ選択）。
- **スコープ設定**：RenoP のプロバイダー設定内のスコープは、空のままにしておく（既定値）か、Cloudflare
  で付与した権限を入力します（カンマまたはスペース区切りに対応。例：
  `memberships.read, user-details.read, offline_access, openid`）。
- **メールとプロフィールの自動同期**：`user-details.read` 権限が付与されている場合、RenoP はサインインや登録時に
  Cloudflare API（`/client/v4/user`）を呼び出して確認済みのメールアドレスとユーザー名を自動取得し、手動のメール確認コード入力を省略します。付与されていない場合は
  `/oauth2/userinfo` の主体識別子（`sub`）で識別し、RenoP の確認コードでメールを検証します。

クライアントを作成後、 **Client ID** とモーダルに一度だけ表示される **Client Secret**（Client Secret POST または Client
Secret Basic を使用する場合；Cloudflare API トークンや Global API Key ではなく OAuth クライアントシークレットである必要があります）を控えます。RenoP
で種類 `cloudflare` を追加し、Client ID、Client Secret（必要な場合）、および対応するトークン認証方式（既定値：
`client_secret_post`）を設定します。前後の空白は自動的に削除されます。

### Stack Exchange

Stack Apps の登録画面（`https://stackapps.com/apps/oauth/register`）を開きます。

情報を入力します：

- **Application Name**：アプリ名。
- **OAuth Domain**：RenoP のホストドメイン（スキームやポートを除いた例：`renop.example`）。
- **Enable Client Side Flow**：チェックを外したままにします。

登録後、 **Client Id**、 **Client Secret**、および **Key**（API 呼び出しと制限に必要なキー）を控えます。RenoP で種類
`stackexchange` を追加し、Client ID、Client Secret、API Key を入力します。`site` の既定値は `stackoverflow` です。

### カスタムプロバイダー

Keycloak、Authentik、Authelia、Casdoor、Dex などの標準 OpenID Connect / OAuth 2.0 サービスでは、種類 `custom` を選択します。

各エンドポイントを設定します：

- **認可 URL**：ユーザーの認可画面へのリダイレクト先。
- **トークン URL**：認可コードをトークンに交換するエンドポイント。
- **ユーザー情報 URL**：ユーザープロファイル JSON を取得するエンドポイント。
- **発行者** と **JWKS URL**：OIDC の署名検証に必要です。
- **スコープ**：スペース区切りのスコープ（例：`openid profile email`）。
- **トークン認証方式**：提供元に合わせて `client_secret_post` または `client_secret_basic` を選択します。

クレームマッピングを設定します：

- `subject`：一意で安定したユーザー識別子の JSON パス（例：`sub` や `id`）。
- `username` / `name`：ユーザー名と表示名。
- `email` / `email_verified`：メールアドレスと検証済みフラグ。

## 設定と認証情報

他のプロバイダーは `server.oauth_providers` に保存されます。次の例では、認証情報を置き換えるまでクライアントを無効にしています。

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

`name` は UTF-8 で最大 80 バイト、`client_id` は最大 512 バイト、シークレットと API キーは最大 4,096 バイトです。`scopes`
は空白区切りの文字列で、最大 1,024 バイトです。エンドポイント URL は最大 2,048
バイトです。組み込みプリセットは公式エンドポイントとフィールド対応を設定します。その他の形式には `custom` を使用してください。

カスタムの `claims` は、`user.id` や `items.0.id` のようなドット区切りの JSON パスと数値の配列インデックスでスカラー値を選択します。パスは最大
128 文字です。`subject` にはプロバイダーの安定した識別子を使い、名前やメールをアカウント識別子として使用しないでください。数値の識別子は精度を維持します。任意のプロフィール項目は省略できます。文字列の
`"true"` ではメールは確認済みになりません。

カスタムの `token_auth` には `client_secret_post`、`client_secret_basic`、`none` を指定できます。PKCE S256 は既定で有効です。
`disable_pkce: true` は、クライアントシークレットを持つカスタムプロバイダーでのみ使用できます。OpenID Connect では
`issuer` と `jwks_url` を両方設定し、`scopes` に `openid` を含めてください。RenoP は ID トークンを必須として検証します。OIDC
を使わない OAuth クライアントでは、認証されたユーザー情報レスポンスを使用します。

OAuth のみを使うカスタムクライアントは、主体識別子のフィールドパスも認証元の境界に含めます。このフィールドを変更した場合は連携し直してください。コミット
`26a1c1a` からこの種類のクライアントを有効にしていた場合も、本ルールへの更新後に再連携が必要です。OIDC クライアントは検証済み
ID トークンの主体識別子を引き続き使います。Cloudflare が提供するのは `sub` のみのため、メールや画像の取り込み操作は表示しません。登録では資料を手入力し、RenoP
のメールコードで確認します。

設定の読み取りでは `client_secret_configured` と `api_key_configured`
が返され、シークレットの値は空です。空欄の書き込みで保存済みの値を保持できるのは、ID、プロバイダーの種類、クライアント
ID、トークンエンドポイントが同じ場合のみです。`clear_client_secret` と `clear_api_key`
で明示的に削除できます。クライアントやエンドポイントを変更した場合は、認証情報を再入力してください。プロバイダーを削除すると新しい認可は停止しますが、ユーザーが解除できるよう既存のアカウント連携は保持されます。

## 登録とアカウント操作

未連携のプロバイダーでログインすると、[アカウント登録](./registration.md)
が始まります。ユーザーが確認してパスワードを設定するまで、アカウントやログインセッションは作成されません。どのプロバイダーにも、10
分の確認期限、プロバイダー登録の待機期間、IP
ごとの上限、宛先ポリシー、ローカルのユーザー名・ニックネームの制限、アバターの容量制限が適用されます。メールが一致しても、既存アカウントが自動で選ばれたり統合されたりすることはありません。

確認待ちのレスポンスには `provider`、`provider_name`、`email_required`、`mail_enabled`、`avatar_available` が含まれます。
`email_required` が true の場合、プロバイダーが未確認の候補アドレスを返していても、ユーザーはアドレスを入力して確認する必要があります。登録コードのエンドポイントには
`provider` と `email` を一緒に送信してください。最終確認でも同じプロバイダー ID と受信した `code`
を送信します。コードの再発行では元の確認期限や失敗回数はリセットされません。メールを利用できない場合、確認が必須のプロバイダーでは登録を完了できません。

プロバイダーが確認済みアドレスを返した場合、そのアドレスは確認中に変更できず、コードは不要です。ユーザー名、ニックネーム、アバターの取り込みは任意です。画像の欠落や容量超過によって登録が取り消されることはありません。名前にはインスタンスの制限が適用され、重複するユーザー名は手動で変更する必要があります。

プロフィール編集画面では、プロバイダーの連携や更新、画像の取り込み、最新の確認済みメールの使用、連携解除ができます。メール確認には新しい認可が必要で、ログイン連携は変更されません。メール確認フィールドのないプロバイダーには、この操作は表示されません。画像を取り込むには、選択した外部
ID
が現在のアカウントに連携されている必要があります。サーバーは解除前に別の主要なログイン方法が残ることを確認します。[二段階認証](./two-step-verification.md)
、アカウント停止、永久閉鎖は外部ログインにも適用されます。

## API

| メソッド | パス                                 | 契約                                                                                 |
|----------|--------------------------------------|--------------------------------------------------------------------------------------|
| GET      | `/api/auth/oauth/providers`          | 設定済みプロバイダーの ID と名前の公開一覧                                           |
| GET      | `/api/auth/oauth/:provider/start`    | `intent=login`、`register`、`link`、`email`、`avatar`。ローカルの `return_to` は任意 |
| GET      | `/api/auth/oauth/:provider/callback` | 一回限りの認可コードとブラウザーに紐づいた状態                                       |
| GET      | `/api/auth/profile/oauth`            | 非公開の連携状態、表示識別子、認可日時、許可された操作                               |
| DELETE   | `/api/auth/profile/oauth/:provider`  | 解除時は `204`。最後のログイン方法の場合は `409` と `oauth_last_login_method`        |
| GET      | `/api/settings/oauth-providers`      | 管理者用の `providers` と `presets`。シークレットは含まない                          |
| PUT      | `/api/settings/oauth-providers`      | 管理者用 JSON `{providers:[...]}` で一覧を置換。本文の上限は 128 KiB                 |

プロフィール操作には現在のブラウザーセッションが必要です。コールバックは `oauth` に安定した結果マーカー、`provider` に ID
を返し、SPA が翻訳してパラメーターを削除します。エラー時にプロバイダーの生のレスポンスは表示しません。セッションのログイン方法は
`oauth:<provider-id>` で、必要に応じて `+totp` または `+passkey` が付きます。

## GitLab の Maven 認可

過去 1 時間以内の GitLab.com 認可により、新規の `io.gitlab.<namespace>` ドメインを自動確認できます。RenoP
が受け入れるのは、本人の名前空間と GitLab のグループ所有者クレームにあるトップレベルのグループです。公開グループへの参加、サブグループの所有、自前の
GitLab インスタンスの ID では、親や GitLab.com の名前空間を認可できません。連携を更新すると所有権の証明が更新されます。ID
ごとに最大 1,001 件の名前空間を保持します。公開プロフィールやグループの説明文による従来の確認も利用できます。

## セキュリティと運用

OAuth コールバックは、有効期間 10 分の HttpOnly Cookie、一回限りのサーバー状態、PKCE を使用します。外部認可の状態は最大
2,048 件保持します。設定が変わると、確認待ちのコールバックと登録は無効になります。OIDC
は署名、発行者、対象、主体、nonce、有効期間、および提供されている場合のトークンハッシュを検証します。対応する署名は RS256 と
ES256 です。プロバイダーのレスポンスは 1 MiB に制限され、リクエストにはタイムアウトがあり、設定済みの外向きプロキシを使用します。

RenoP は、プロバイダーの種類、クライアント、エンドポイント、検証済みの発行者から決まる認証元に、安定した主体識別子を紐づけて保存します。認証元を変更しても、既存の連携が別の
ID サービスに移ることはありません。ログイン用のアクセストークンは保持しません。保護された画像に必要なトークンは、登録確認が完了または期限切れになるまで、確認待ち登録の中だけに暗号化して保持できます。この一時値は非公開の
`mfa_encryption_key` で保護され、ブラウザーには返されません。

アカウントの閉鎖は、すべての外部連携を一つのトランザクションで解放します。解放された ID
は別の有効なアカウントに連携できますが、閉鎖されたユーザー名は永久予約、メールは 14 日間保持、操作履歴は 30
日間保持のままです。遅れて到着したコールバックや更新で、閉鎖済みアカウントの連携を再作成することはできません。

新しい連携を確定するには、外部 ID と取得したすべての連絡先が同時に利用可能である必要があります。解放された ID
でも、他のアカウントが保持するメールの所有権を回避できません。確認、削除、保持の規則は[ログイン用メールの別名](./email-verification.md)
を参照してください。
