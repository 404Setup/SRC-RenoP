---
title: メール API の権限
order: 7
category: 設定
description: プロバイダーの認証、確認済みの API 仕様、メール連携の要件
---

# メール API の権限

[メール配信](mail.md)の 8 種類の API プロバイダーと SMTP OAuth を説明します。
エンドポイントと権限名は、2026-09-08 に本文の公式資料で確認しました。
RenoP は HTTP を直接使用するため、プロバイダーの SDK は不要です。

## 権限一覧

設定した送信者に送信権限を付与し、使用する機能の照会権限を追加します。
送信要求の受理は配信完了を意味しません。メールボックスの読み取り権限では、認証情報の保有者がメール内容にアクセスできる場合もあります。

| プロバイダー        | 送信                                         | 状態                                             | 割り当て / 残高                                               |
|---------------------|----------------------------------------------|--------------------------------------------------|---------------------------------------------------------------|
| Cloudflare          | アカウントを限定した Email Sending: Edit     | 初回応答                                         | RenoP では該当する照会なし                                    |
| Microsoft Graph     | `Mail.Send`                                  | 拡張プロパティ照会に `Mail.Read`                 | 該当する照会なし                                              |
| Amazon SES v2       | `ses:SendEmail`                              | `ses:GetMessageInsights`                         | `ses:GetAccount`、残高照会なし                                |
| SendGrid            | `mail.send`                                  | `messages.read` と Email Activity 履歴アドオン   | `user.credits.read`、現金残高照会なし                         |
| Gmail               | `https://www.googleapis.com/auth/gmail.send` | `https://www.googleapis.com/auth/gmail.metadata` | 送信可能数や残高の照会なし                                    |
| Alibaba Direct Mail | `dm:SingleSendMail`                          | `dm:SenderStatisticsDetailByParam`               | `dm:DescAccountSummary`、任意で `bss:DescribeAcccount`        |
| Tencent SES         | `ses:SendEmail`                              | `ses:GetSendEmailStatus`                         | 任意で `finance:DescribeAccountBalance`、送信可能数の照会なし |
| Feishu / Lark       | `mail:user_mailbox.message:send`             | `mail:user_mailbox.message:readonly`             | 該当する照会なし                                              |

OAuth では初回のユーザー認可が必要で、自動更新にはリフレッシュトークンも必要です。
RenoP は保存済みの認証情報を更新します。メール設定には対話的な OAuth コールバックはありません。
登録したアプリの認可コードフローと正確な登録済みリダイレクト URI を使用し、取得したリフレッシュトークンを非公開で保存してください。

## Cloudflare Email

アカウントで Email Sending を有効にし、送信ドメインと必要な DNS レコードを設定します。
対象アカウントに限定した **Email Sending: Edit** トークンを `api_key` に、その所有アカウントを `account_id` に設定します。
Email Routing 権限では外部への送信を許可できません。ドメインや DNS の管理権限は、実行時の送信権限とは別です。

`https://api.cloudflare.com/client/v4` の `POST /accounts/{account_id}/email/sending/send` を使用します。
構造化したアドレスとテキスト/HTML を送信し、`message_id`、`delivered`、`permanent_bounces`、`queued`、`suppressed_recipients`
を読み取ります。
キューに入った結果は `queued_provider` のままです。Cloudflare の別のイベント購読は使用しません。
[設定と権限](https://developers.cloudflare.com/email-service/get-started/send-emails/)
および[送信仕様](https://developers.cloudflare.com/api/resources/email_sending/methods/send/)を参照してください。

## Microsoft Graph

メールボックスが属するクラウドで Entra アプリを登録します。個人用 Outlook.com は委任認可を使用し、
Microsoft 365 組織では委任権限またはアプリケーション権限を使用できます。
委任では `Mail.Send Mail.Read offline_access` を認可し、`client_id`、必要な `client_secret`、`refresh_token` を保存して
`mailbox: me` を指定します。
対応するグローバルアプリ登録ではテナント `common` を使用できます。組織のポリシーによって管理者承認が必要です。

アプリケーションアクセスでは、管理者の同意を得たアプリケーション権限 `Mail.Send` と `Mail.Read` が必要です。
実際のテナント ID、クライアント ID、シークレット、対象 `mailbox` のユーザー ID またはアドレスを指定します。
RenoP は選択した Graph リソースの `/.default` スコープで `client_credentials` を要求します。
アプリケーショントークンで `/me` は使用できません。Exchange の対応するアクセス制御で対象メールボックスを制限してください。

| クラウド                                     | API ベース                                     | トークン発行元                      |
|----------------------------------------------|------------------------------------------------|-------------------------------------|
| グローバル / Outlook.com / Microsoft 365 GCC | `https://graph.microsoft.com/v1.0`             | `https://login.microsoftonline.com` |
| 米国政府 L4 / GCC High                       | `https://graph.microsoft.us/v1.0`              | `https://login.microsoftonline.us`  |
| 米国政府 L5 / DoD                            | `https://dod-graph.microsoft.us/v1.0`          | `https://login.microsoftonline.us`  |
| 中国 / 21Vianet                              | `https://microsoftgraph.chinacloudapi.cn/v1.0` | `https://login.chinacloudapi.cn`    |

送信には `POST /me/sendMail` または `POST /users/{mailbox}/sendMail` を使用し、設定した `from` で送信済みコピーを保存します。
状態照会は RenoP の拡張追跡プロパティで送信済みアイテムを絞り込みます。`Mail.ReadBasic` はこのプロパティを対象に含みません。
別のメールボックスからの委任送信には `Mail.Send.Shared`、対応する共有読み取り権限、Exchange の Send As/Send on Behalf
が必要で、直接アクセスには Full Access も必要です。
HTTP 202 は受理を、保存済みメッセージの発見は `sent` を示します。
[sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail)、[拡張プロパティ権限](https://learn.microsoft.com/en-us/graph/api/singlevaluelegacyextendedproperty-get?view=graph-rest-1.0)、
[共有送信](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user)、[クラウド別アドレス](https://learn.microsoft.com/en-us/graph/deployments)
を参照してください。

## Amazon SES

選択したリージョンで送信者 ID を検証します。SES サンドボックス外への送信には本番アクセスが必要です。
IAM アクセスキーとシークレットを設定します。一時的な認証情報にはセッショントークンも必要で、有効期限前に管理者が更新します。
SES SMTP 認証情報を SES API 認証情報として使用することはできません。

許可した ID リソースに `ses:SendEmail` を付与します。`ses:GetAccount` と `ses:GetMessageInsights` は ID
単位のリソース制限に対応しないため、リソースを `*` にします。
メッセージインサイトには対応する Virtual Deliverability Manager 機能が必要で、通常の送信とは別料金です。
RenoP はサービス `ses` と設定したリージョンを使い、AWS SigV4 で署名します。

`POST /v2/email/outbound-emails` は `MessageId` を返し、`GET /v2/email/account` は `SendingEnabled` と直近 24
時間の送信枠を返します。
`GET /v2/email/insights/{message_id}/` は受信者のイベントを返します。強制送信でも SES の上限は適用されます。
[IAM 操作](https://docs.aws.amazon.com/service-authorization/latest/reference/list_sesv2.html)、
[アカウント照会](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetAccount.html)、
[メッセージインサイト](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetMessageInsights.html)、[リージョン別アドレス](https://docs.aws.amazon.com/general/latest/gr/ses.html)
を参照してください。

## Twilio SendGrid

送信者またはドメインを検証し、必要に応じて `mail.send`、`messages.read`、`user.credits.read` を含む制限付き API キーを作成します。
Email Activity API には追加の履歴製品が必要です。送信専用キーや送信プランだけでは状態を照会できません。
EU 送信には対応する Pro 以上のプラン、EU サブユーザーと EU IP が必要です。EU プリセットにそのサブユーザーのキーを指定します。

選択した `/v3` ベースの下で `POST /mail/send`、`GET /messages`、`GET /user/credits` を呼び出します。
応答の `X-Message-Id` は受信者別のアクティビティ ID の接頭辞です。状態照会では受信者も厳密に照合します。
照会値は `msg_id LIKE "submission-id%"` のように二重引用符で囲みます。
Credits はメールの利用枠であり、引き出し可能な残高ではありません。
[API キー権限](https://www.twilio.com/docs/sendgrid/api-reference/api-key-permissions)、
[アクティビティの利用条件](https://www.twilio.com/docs/sendgrid/api-reference/email-activity/filter-all-messages)、
[照会構文](https://www.twilio.com/docs/sendgrid/for-developers/sending-email/getting-started-email-activity-api)、
[クレジット](https://www.twilio.com/docs/sendgrid/api-reference/users-api/retrieve-your-credit-balance)、[地域別送信](https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send)
を参照してください。

## Google Gmail

OAuth クライアントの Google Cloud プロジェクトで Gmail API を有効にし、同意画面を設定します。
メールボックスのユーザーから `https://www.googleapis.com/auth/gmail.send` と
`https://www.googleapis.com/auth/gmail.metadata` の認可を取得します。
`access_type=offline` を要求し、初回のリフレッシュトークンが発行されない場合は再同意を取得します。
Testing のままの外部アプリでは、7 日で失効するリフレッシュトークンが発行される場合があります。公開と制限付きスコープの検証要件はアプリによって異なります。

クライアント ID、シークレット、リフレッシュトークンを保存し、RenoP が `https://oauth2.googleapis.com/token` で交換します。
送信者は認可したメールボックスまたは許可済みの送信エイリアスでなければなりません。
この接続機能ではサービスアカウント JSON キーやドメイン全体の委任 JWT 生成は実装していません。

`https://gmail.googleapis.com/gmail/v1` の `POST /users/me/messages/send` は base64url MIME を受け取り、メッセージ ID
を返します。
`format=minimal` を付けた `GET /users/me/messages/{message_id}` で `SENT` ラベルを確認します。
確認できるのは `sent` で、配信完了ではありません。API リクエスト枠とメールボックスの送信上限も別です。
[送信権限](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send)、
[メタデータ](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/get)、[OAuth 設定](https://developers.google.com/identity/protocols/oauth2/web-server)
を参照してください。

## Alibaba Cloud Direct Mail

Direct Mail を有効にし、選択したリージョンでドメインを検証して送信者アドレスを設定します。
RAM キーに `dm:SingleSendMail`、`dm:DescAccountSummary`、`dm:SenderStatisticsDetailByParam` を付与します。リソースは `*`
です。
任意の残高校正には **`bss:DescribeAcccount`** を付与します。連続する 3 つの `c` は公式の綴りです。
残高 API の操作名は `QueryAccountBalance`、バージョンは `2017-12-14` で、`dm:` 権限ではありません。

RenoP は署名付き RPC POST と Direct Mail バージョン `2015-11-23` を使用します。
`SingleSendMail` は `EnvId`、`DescAccountSummary` は無料枠とアカウント状態を返します。
公開配信統計には信頼できるメッセージ ID がないため、受信者だけで結果を推定せず、照会後は `unknown` とします。
送信者表示名は 15 文字以内にします。課金エンドポイントは送信リージョンとは別に商用アカウントの地域に合わせ、料金通貨を応答通貨に合わせてください。

[送信と制限](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-singlesendmail)、
[送信枠の権限](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-descaccountsummary)、
[配信統計](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-senderstatisticsdetailbyparam)、
[残高の権限](https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-queryaccountbalance)、[接続先](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint)
を参照してください。

## Tencent Cloud SES

送信ドメインとアドレスを検証し、CAM の `ses:SendEmail` と `ses:GetSendEmailStatus` を付与します。
公式 CAM ポリシーでは、両方の操作のリソースは `*` です。
任意の残高アクセス権限は `finance:DescribeAccountBalance` で、署名上の API サービス名は `billing` です。
SecretId を `api_key`、SecretKey を `api_secret` に設定し、一時認証の場合は Token を `session_token` に設定します。

通常のアカウントには承認済みプロバイダーテンプレートが必要です。その正の ID を `tencent_template_id` に設定します。
次のテキスト変数を使用する HTML テンプレートを作成して審査に提出し、承認後に ID を使用します。

```html
<div style="padding:24px;background:#f3f5f8;font-family:Arial,sans-serif">
  <div style="max-width:600px;margin:auto;padding:24px;background:white;border:1px solid #dfe5ef;border-radius:20px">
    <p style="color:#3158c9;font-weight:bold">RenoP</p>
    <h1>{{subject}}</h1>
    <div style="white-space:pre-wrap;overflow-wrap:anywhere">{{text}}</div>
  </div>
</div>
```

RenoP は件名と完全なプレーンテキスト通知を渡し、置換前に HTML 文字をエスケープします。
変数はテキスト内だけに置き、属性やスクリプトには置かないでください。`TemplateData` JSON 全体の上限は UTF-8 で 800
バイトです。超過すると送信前に失敗し、切り詰めません。
この例は審査通過を保証しません。テンプレートモードの外観は承認済み HTML に従います。
ID 0 は、以前からカスタム本文の特別承認を得ているアカウント向けに `Simple` を保持するもので、新たな権限を付与しません。

`SendEmail` と `GetSendEmailStatus` はバージョン `2020-10-02`、サービス `ses` の TC3 署名を使用します。
メッセージ ID と受信者を照合し、受理、配信、拒否、一時的な遅延を区別します。
`DescribeAccountBalance` はバージョン `2018-07-09` です。利用可能残高をセントから通貨の百万分の一単位に変換します。
中国版と国際版にはそれぞれのアカウント体系と通貨があります。
[SendEmail](https://intl.cloud.tencent.com/document/product/1084/39408)、
[テンプレートと状態](https://intl.cloud.tencent.com/document/product/1084/39418)、
[SES CAM 操作](https://intl.cloud.tencent.com/document/product/598/57150)、
[課金権限](https://cloud.tencent.com/document/product/555/61542)、[残高 API](https://cloud.tencent.com/document/api/555/20253)
を参照してください。

## Feishu と Lark Mail

Mail が有効なメールボックスを持つ組織で、カスタムアプリを作成して公開します。
`mail:user_mailbox.message:send`、`mail:user_mailbox.message:readonly`、`offline_access` を付与します。
送信ユーザーをアプリの利用対象に含め、セキュリティ設定に更新スイッチがある場合は有効にして公開し、ユーザーの同意を取得します。
ここで使用する送信と配信状態の操作には **ユーザー**トークンが必要で、テナントトークンでは認可できません。

アプリ ID、シークレット、リフレッシュトークンと、メールアドレスまたは `me` を設定します。
Feishu と Lark はアプリ登録、認証情報、API ベースが別です。
接続先は `https://open.feishu.cn/open-apis` と `https://open.larksuite.com/open-apis` です。
RenoP は現在、互換性のある `POST /authen/v2/oauth/token` を使用し、更新されたトークンを送信前に永続化します。

`POST /mail/v1/user_mailboxes/{mailbox}/messages/send` はパディング付き base64url MIME を受け取り、`data.message_id`
を返します。
`GET /mail/v1/user_mailboxes/{mailbox}/messages/{message_id}/send_status` は受信者別の詳細を返します。
4 は配信済み、3 と 6 は失敗で、0、1、2、5 は照会を継続します。
配信済み状態は、受信者への配信結果によってのみ確定します。
[送信](https://open.feishu.cn/document/server-docs/mail-v1/user_mailbox-message/send)、
[配信状態](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status)、
[Lark の配信状態](https://open.larksuite.com/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status)、[トークン更新](https://open.feishu.cn/document/authentication-management/access-token/refresh-user-access-token)
を参照してください。

## SMTP OAuth

Gmail SMTP には `https://mail.google.com/` を認可します。Gmail API の送信専用スコープでは不十分です。
Outlook.com/Microsoft 365 SMTP には `https://outlook.office.com/SMTP.Send` と `offline_access` が必要です。
Microsoft 365 の組織やメールボックスのポリシーに応じて SMTP AUTH も有効にします。
SMTP ユーザー名をメールアドレスにし、対応するクライアントと更新用認証情報を設定します。

自動 SMTP 更新は委任認証を使用し、`SMTP.SendAsApp` によるアプリケーショントークン取得は行いません。
外部管理のアクセストークンも設定できますが、その更新は管理者が行います。
[Google SASL OAuth](https://developers.google.com/workspace/gmail/imap/xoauth2-protocol)
と [Microsoft SMTP OAuth](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth)
を参照してください。

## 検証と運用

アカウントを保存し、管理画面からテストメールを送信して最終状態と校正結果を確認します。
管理する受信箱で実際の受信を確認してください。キュー登録の 202 応答は要求の受理だけを示します。
状態権限が不足すると受理後に `unknown` となる場合があります。照会失敗だけを理由に再送しないでください。
グローバルログのプロバイダーコードを確認し、不足する権限だけを追加します。

公式資料とローカル HTTP 契約テストの確認では、実際の認証情報、契約、送信ドメインや地域アカウントの動作までは証明できません。
今回の確認では実プロバイダーの認証情報を使用していません。実際の送信、送信枠、残高、配信照会は導入先のアカウントで検証する必要があります。
未取得の上限は[メール配信](mail.md)の規則に従い、校正失敗時は取得済みの上限を保持します。
