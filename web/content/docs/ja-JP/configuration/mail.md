---
title: メール送信
order: 6
category: 設定
description: メールプロバイダー、永続キュー、クォータ、料金、テンプレートと管理 API
---

# メール送信

**設定 → サービス**で送信アカウントを追加し、プリセットと認証情報を設定して保存します。
インスタンスの公開 HTTPS URL を設定してからメールを有効にします。変更は再起動せずに以後のキュー処理へ適用されます。

## 設定

```yaml
mail:
  enabled: false
  public_url: https://packages.example.com
  site_name: RenoP
  template_style: card
  delay: {value: 5, unit: second}
  manual_rate: {limit: 1, interval: {value: 2, unit: minute}}
  account_rate: {limit: 50, interval: {value: 1, unit: minute}}
  calibration: {value: 5, unit: minute}
  list_mode: blacklist
  use_disposable_blacklist: false
  addresses: []
  accounts:
    - id: primary
      name: Main mailbox
      enabled: true
      provider: smtp
      preset: smtp-custom
      scenes: ["*"]
      from: noreply@example.com
      from_name: RenoP
      smtp_host: smtp.example.com
      smtp_port: 587
      smtp_security: starttls
      username: noreply@example.com
      password: ""
      quota: {limit: 0, period: month}
      force_send: false
      overage: {limit: -1, period: month}
      balance_micros: null
      fetch_balance: false
      pricing:
        currency: USD
        rounding: proportional
        tiers:
          - {up_to: 0, amount_micros: 100000, batch_size: 1000}
```

有効なアカウントが 1 つなら、すべてのシーンに使用します。複数ある場合は明示的な割り当てが唯一の `*` 既定アカウントより優先されます。
同じシーンを複数の有効アカウントに割り当てることはできません。未割り当てのシーンには送信元がありません。
アカウントの `id` は固定です。表示名を変更しても集計は保持されます。削除したアカウントの未送信メールは、次の処理時にキャンセルされます。
サービスまたはアカウントを無効にすると送信は停止しますが、キュー内メールの有効期限は延長されません。

`delay` は秒、分、時間に対応し、0 は追加待機なしを意味します。送信は引き続き逐次処理されます。
`manual_rate` はテストメールを含む IP ごとの手動要求に適用され、期間は分、時間、日です。
`account_rate` は送信アカウントごとの自動・手動の試行を含み、期間には秒も指定できます。
上限と期間の値は正数です。既定値は IP ごとに 2 分間で手動要求 1 回、アカウントごとに 1 分間で送信試行 50 回です。

`list_mode` は `blacklist` または `whitelist` です。キュー追加前と送信直前に確認します。国際化ドメインの Unicode 表記と Punycode 表記は同じように一致し、末尾のドットは無視します。

| ルール | 一致する範囲 |
|---|---|
| `person@example.com` | 完全なメールアドレス |
| `@example.com` | 完全一致のプロバイダードメイン。サブドメインは除外 |
| `.com` または `.example.com` | 基本ドメインとサブドメインを含む DNS 接尾辞 |

`use_disposable_blacklist: true` で、拒否ルールに内蔵の一時メールリストを追加できます。既定では無効で、許可リストモードでは無視します。現在のスナップショットは [disposable/disposable-email-domains](https://github.com/disposable/disposable-email-domains) と [disposable-email-domains/disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains) の75,627ドメインを統合し、国や所有者で絞り込みません。登録ドメインとそのサブドメインを、外部サービスに接続せず拒否します。

リストはリリース時点のスナップショットで、新しいプロバイダーをすべて網羅するものではありません。カスタムルールで補完できます。ビルド時に `scripts/update-disposable-domains.ps1` で固定したリビジョンとチェックサムから、Git 管理対象外の `internal/mail/data/` を自動生成します。検証済みのローカルデータはオフラインで再利用できます。ライセンスは `THIRD_PARTY_NOTICES.md` を参照してください。

## プロバイダーと接続先

| プロバイダー | プリセットと認証情報 | リモート機能 |
| --- | --- | --- |
| SMTP | 平文 SMTP、暗黙 SSL/TLS、必須 STARTTLS。パスワードまたは対応 OAuth | ローカル推定、SMTP 受付応答 |
| Cloudflare Email | アカウント ID、API トークン、グローバル REST 接続先 | 初回応答の配信、拒否、待機結果 |
| Microsoft Graph | Outlook.com、Microsoft 365/Entra。グローバル、米国政府 L4/L5、中国 | 送信済みフォルダーの状態 |
| Amazon SES | リージョン別 IPv4、デュアルスタック、利用可能な FIPS。アクセスキー、シークレット、任意の一時トークン | 送信クォータ、メッセージの詳細 |
| Twilio SendGrid | グローバルと EU の接続先、API キー | クレジット、Email Activity の状態 |
| Google Gmail | Gmail と Workspace、委任 OAuth | 送信済みラベルの状態 |
| Alibaba Cloud Direct Mail | 杭州、シンガポール、バージニア、フランクフルトの公開・VPC 接続先。アクセスキーとシークレット | 無料枠、残高、正確に関連付けできない配信統計 |
| Tencent Cloud SES | 中国版と国際版の送信・請求 API。アクセスキーとシークレット | 残高、受信者別の配信状態 |
| Feishu / Lark Mail | Feishu と Lark の接続先、委任ユーザー OAuth | 受信者への配信状態 |

リージョンの対応範囲は [SES 接続先一覧](https://docs.aws.amazon.com/general/latest/gr/ses.html)と [Direct Mail 接続先一覧](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint)を参照してください。
Direct Mail の VPC 接続先は同一リージョン内の接続が必要です。廃止されたシドニーの接続先は含みません。
SMTP プリセットは Gmail、Outlook.com、Microsoft 365、QQ、NetEase 163/126、Cloudflare、SendGrid、SES、Direct Mail、Tencent、Feishu を含みます。
接続先と料金は編集可能です。プリセット変更で既定値を読み込み、プロバイダー変更では以前の認証情報を流用しません。

手動クォータと追加枠の上限は保持します。プリセットの通貨が変わる場合、残高を別通貨として扱わず、手動残高を空欄にします。

## 認証情報と権限

正確なスコープ、IAM/CAM/RAM 操作、トークン取得、利用条件、確認済みの仕様は[メール API の権限](mail-api-permissions.md)を参照してください。
Tencent API の通常アカウントには `tencent_template_id` による承認済みテンプレートの指定が必要です。`Simple` のカスタム本文は従来の制限付き機能です。

`smtp_security` は `plain`、`tls`、`starttls` です。既定ポートは暗黙 TLS が 465、それ以外が 587 です。
TLS は証明書とサーバー名を検証します。STARTTLS を選ぶと暗号化への移行が必須になり、平文 SMTP は明示的に選択する必要があります。
必要に応じてアプリパスワードを使用してください。OAuth SMTP は Gmail と Microsoft の更新トークンに対応します。アクセストークンのみの場合は管理者が更新します。

Graph の委任アカウントは `client_id`、任意の `client_secret`、`refresh_token` を使用します。個人アカウントのテナントは `common` を指定できます。
アプリケーション権限は Entra テナント ID とクライアント認証情報を使用し、`mailbox` にユーザー ID かメールアドレスを指定します。委任権限では `me` を使用できます。
送信には `Mail.Send`、送信済み照会には適切な `Mail.Read` 権限が必要です。アプリケーション権限には管理者の同意が必要です。
[Graph sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail)と[各国クラウドへの展開](https://learn.microsoft.com/en-us/graph/deployments)を参照してください。

Gmail は委任送信とメッセージのメタデータ権限が必要です。Feishu/Lark はユーザートークンと送信・読み取り権限が必要です。
RenoP は OAuth 更新を逐次実行し、更新トークンが変更された場合は送信前に永続化します。
API フィールドは `api_key`、`api_secret`、任意の `session_token`、`account_id`、`region`、`endpoint`、任意の `billing_endpoint` です。
送信元アドレスはプロバイダーの許可が必要です。アカウント ID、メールボックス ID、API キーは別の値です。

## クォータと料金

`quota.limit` はローカルの送信枠です。負数は無制限、0 または省略は自動取得、正数は手動設定です。
リモートの枠が取得できない場合、自動モードは初期状態で無制限になります。補正に失敗しても既知の枠を保持し、新たな枠を付与しません。
手動の期間は UTC の時間、日、月曜始まりの週、月です。4 種類の期間の使用量を独立して保持します。

`force_send` を有効にすると、通常の枠を使い切った後で追加枠を使用できます。
`overage.limit` の負数は追加枠無制限、0 は追加送信禁止です。期間は `hour`、`day`、`week`、`month` に対応します。
プロバイダーの強制上限は引き続き適用され、SES の技術的な送信上限は追加料金で回避できません。
クォータ API がない場合は、契約に含まれる枠を手動で設定します。

`pricing.tiers` の `up_to` は累積通数の昇順です。最後の段階だけ 0 を指定でき、上限なしを意味します。
各段階は `batch_size` 通あたり `amount_micros` を課金します。1 通単位ならバッチ数を 1 にします。
`rounding: proportional` はバッチ料金を比例配分し、`batch` はバッチの最初の 1 通で全額を課金します。
金額は 3 文字の `currency` 通貨の百万分の一で表し、1000000 が通貨 1 単位です。画面では通常の金額で表示します。

料金プリセットの確認日は 2026-09-08 です。[Cloudflare](https://developers.cloudflare.com/email-service/platform/pricing/) の追加送信は 1000 通あたり 0.35 USD、[SES](https://aws.amazon.com/ses/pricing/) の送信は 1000 通あたり 0.10 USD です。
[SendGrid の公開料金](https://sendgrid.com/content/dam/sendgrid/global/en/other/sendgrid-pricing/twi121--sendgrid-pricing-pdf-st1.pdf)から Essentials 50K と Pro 100K の編集可能な既定値を提供します。EU 送信には対応プランが必要です。
[Direct Mail](https://www.alibabacloud.com/help/en/direct-mail/billing-methods) は 1000 通あたり 0.29 USD、[Tencent 中国版](https://cloud.tencent.com/document/product/1288/47930)は 1 通 0.0019 CNY、[国際版](https://www-sg.tencentcloud.com/document/product/1084/39335)は 1 通 0.00028 USD です。
Graph、Gmail、Feishu/Lark の限界費用の推定は 0 ですが、契約の上限は有効です。
契約基本料、税、添付データ、オプション料金は含みません。実際の契約に合わせて編集してください。

`balance_micros` の空欄は残高不明です。既知の残高が 0 以下、または次の追加送信料金に足りない場合は停止します。
`fetch_balance` は対応プロバイダーの残高を設定通貨で補正します。取得不能は残高 0 として扱いません。
送信前に枠と追加料金を予約します。認証情報や送信元の設定が無効な場合、送信要求前の接続失敗では枠を消費しません。
受信者の拒否や結果不明の送信など、それ以外の試行は枠を消費します。明確に非課金の失敗は一度だけ返却し、試行回数の制限は保持します。
後の配信確認で失敗が分かっても、新しいリモート補正がその調整を含む場合は枠を二重に戻しません。

`calibration` は既定で 5 分、単位は分、時間、日です。空欄、0、負数ではリモートの枠・残高照会を無効にします。
補正の間はローカルで減算します。リモート補正を有効にしたアカウントは、初回送信前に最初の照会を行います。
公開の枠・残高 API がないプロバイダーは、ローカル推定と手動設定を使用します。

## キューと配信状態

送信、OAuth 更新、リモート補正、配信確認を 1 つの逐次ワーカーで処理します。既定では送信試行の完了後に 5 秒待機します。
キューと集計は再起動後も残ります。データベースのリースが複数ワーカーによる同時送信を防ぎます。
結果の保存前に中断した送信は `unknown` になり、自動再送しません。

`accepted` はプロバイダーによる受付、`sent` は送信済みフォルダーまたはメッセージ状態の確認成功です。
`delivered` には明示的な配信結果が必要です。待機、停止、確認、失敗、期限切れ、キャンセル、不明を区別します。
SES と SendGrid の状態照会には対応機能と権限が必要です。Graph と Gmail の送信済み状態は受信者への配信を保証しません。
Feishu/Lark は受信者別の `send_status` API で配信、拒否、処理待ちを区別します。
Direct Mail の公開統計にはメッセージ ID がないため、照会後も `unknown` とし、別のメールの結果を誤って関連付けません。
Cloudflare は初回応答をそのまま使用し、未提供の個別照会 API は呼び出しません。

送信後の照会は上限付きの待機間隔で順次実行します。照会の反復失敗や期限切れは不明状態とグローバルログを生成します。
プロバイダーの失敗コードは管理者ログに保存します。公開状態には宛先、本文、認証情報、未加工の診断を含めません。

## テンプレートと通知

`template_style` は `card`、`compact`、`notice` です。テンプレートはサービス画面と共通の落ち着いた背景色、角丸カード、カプセル型ボタン、明暗の配色を使用します。メールには受信者のアカウントに保存された言語を使用します。新規登録や言語未設定の場合は操作元ページの `Accept-Language` を使用し、既定は `en-US` です。画面と同じ 12 言語に対応し、キューへの追加時に言語を確定します。旧グローバル設定 `locale` は無視され、設定画面からの手動選択はありません。
各スタイルにプレーンテキスト、エスケープ済み変数、設定インスタンスに限定した HTTPS リンクが含まれます。

```text
registration_verify, registration_success, password_reset, password_changed,
email_verify, email_changed, quota_changed, review_status, review_requested,
permission_changed, account_banned, account_unbanned, collaboration_invitation,
super_team_invitation, pending_reviews, unusual_login, security_changed,
account_retired, notification, test
```

パスワード・Passkey・権限の変更、停止と解除、ユーザークォータの上書き、審査、招待、受信メッセージは永続イベントを使用します。
通知は重複排除され、メールを初めて有効にした際に古い監査履歴を再送しません。
新規ネットワークの通知は前回ログインの IPv4 /24 または IPv6 /56 と比較します。地理的位置の判定ではありません。
アカウント関連メールは送信前に存続状態と現在のセキュリティメールを再確認し、アカウント廃止時にはそのキューを削除します。

## 管理 API

```http
GET /api/settings/mail
PUT /api/settings/mail
GET /api/settings/mail/presets
POST /api/settings/mail/test
GET /api/settings/mail/accounts/:id
GET /api/settings/mail/jobs?limit=20&offset=0&status=failed
GET /api/settings/mail/templates/:scene?style=card
GET /api/auth/mail/:id
```

設定 API は管理者権限が必要です。JSON はメール設定全体を置き換え、本文の上限は 1 MiB です。
GET と成功した PUT は秘密値を返さず、アカウント ID と設定済みフィールド名の対応を `secrets_configured` で返します。
同じプロバイダーの空の認証文字列は既存値を保持します。`clear_secrets: {"primary":["password"]}` で明示的に削除できます。
書き込み専用フィールドは `password`、`api_key`、`api_secret`、`session_token`、`client_secret`、`access_token`、`refresh_token` の 7 つです。
永続暗号化キーはこの API では設定も取得もできません。

テスト送信は保存済み設定を使用し、次の JSON を受け取ります。

```json
{"account_id":"primary","to":"receiver@example.com"}
```

追加成功は HTTP 202 で返り、配信完了を意味しません。

```json
{"id":"opaque-job-id","status":"queued","ticket":"private-status-capability"}
```

返された `X-Renop-Mail-Ticket` ヘッダーか、所有者のログインセッションで状態 API を照会します。
チケットは非公開にし、URL に含めないでください。所有者やチケットが無効なら 404、ストレージが利用不能なら 503 です。
アカウント状態は各期間の使用量、試行回数、残枠推定、残高、追加料金、最終補正結果を含みます。
一覧は `limit` 1～50、`offset` 0～10000、任意の完全一致 `status` に対応します。プレビューは件名、HTML、テキストを返します。

SMTP の保存済み認証情報は、ホストとユーザー名が両方変わらない場合のみ保持します。

## ストレージと上限

アカウントは最大 64、宛先リストは 1000 件、料金段階は各アカウント最大 20 です。
期間は最大 1 年、クォータと料金段階の数量は最大 10 億通です。
テキストと HTML の合計、およびプロバイダー応答はそれぞれ 128 KiB までです。1 タスクは宛先 1 件、有効期限は最大 24 時間です。
実行中タスクは最大 2048、全記録は最大 12048 です。保守処理は完了履歴を約 8000 件に縮小し、7 日を超えた記録を削除します。
期限切れ IP 集計を削除し、削除済み送信アカウントの状態は 24 時間後に清掃します。

本文と更新認証情報は、非公開設定ファイルの永続 `mail.encryption_key` で暗号化します。
このキーをデータベースと一緒にバックアップしてください。紛失・置換すると保留メールやアカウント状態を復号できません。
ワーカーが確定したタスクの HTML とテキストは削除します。中断した送信の暗号化本文は履歴清掃まで残ります。
ログと状態 API はメール本文や宛先を公開しません。
