---
title: チケット API
order: 13
category: API リファレンス
description: チケットはフィードバック、提案、通報、所有権移管、公開承認を一つの永続的な処理に統合します。従来の審査センターを置き換え、既存のタスク ID と保留中の公開内容を維持します。
---

# チケット API

チケットはフィードバック、提案、通報、所有権移管、公開承認を一つの永続的な処理に統合します。従来の審査センターを置き換え、既存のタスク ID と保留中の公開内容を維持します。

## 対象と認証情報

すべての経路で有効なブラウザーの `renop_session` Cookie が必要です。Basic 認証、Bearer API Token、この Cookie のないセッショントークンは拒否されます。入口は `/account/tickets` です。旧 `/account/reviews` リンクもここへ移動します。

リポジトリのモデレーターはチーム段階を含め、担当範囲の全状態を確認できます。システム管理者は全範囲を確認できます。T3/T4 メンバーは担当チームの移管と作成だけを処理し、チーム承認は引き続きリポジトリ承認より先です。申請履歴は不変のアカウント ID に結び付きます。

権限を持つ担当者はチケット内で同僚の身元を確認できます。申請者には `assignee`、`escalated_by`、`decided_by` を返しません。通報対象のアカウントは管理者でも自身への通報を閲覧・処理できません。対象者には通報者の身元やチケットへのアクセスを提供しません。

保留通知は現在の承認段階に従います。最終結果は申請者の言語でメッセージと、有効ならメールで送信します。通報が `upheld` で完了した場合のみ対象者へ別の通知を送り、リソースと結果だけを含めます。チケット ID、通報者、担当者、非公開本文は含めません。棄却と撤回では対象者へ通知しません。既存メールシーン ID は互換性を維持します。

通報の監査イベントにはアカウント、担当者、セッション、IP の身元情報を記録せず、担当者の帰属は閲覧権限のあるチケット内に保持します。

## フィードバック・提案・通報を送信

POST /api/tickets は `kind`（`feedback`、`suggestion`、`report`）、`title`（1–160 文字）、`body`（1–8000 文字）、任意の `repository` を受け付けます。JSON 本文は最大 48 KiB。サポート申請はアカウントごとに保留 16 件、24 時間で新規 24 件までで、すべての処理に共通する全体の保留上限は 4096 件です。

通報には `target` の `format`、`repository`、`name`、任意の `version` も必要です。形式は `user`、`superteam`、`maven-domain`、`maven`、`cargo`、`npm`、`docker`。全体リソースではリポジトリを省略します。パッケージはリポジトリ形式と一致し、現在の読み取り権限が必要です。他のユーザー、閲覧可能なパッケージやバージョン、公開ドメイン、チームを通報できます。自身のリソース、非公開の対象、同じ保留中の通報は拒否されます。

作成は `201`、チケット、`Location` を返し、対象所有者を不変のアカウント ID で保存します。`upheld` の記録だけではアカウント停止やパッケージロックを実行しません。処分成立を記録する前に、既存の管理操作で必要な処置を行ってください。

```json
{"kind":"report","title":"Package report","body":"Please investigate this version.","target":{"format":"npm","repository":"npm","name":"@platform/tool","version":"1.0.0"}}
```

## 担当・エスカレーション・解決

POST /api/tickets/{id}/action は `action` として `claim`、`release`、`escalate`、`process`、`complete`、`close` を受け付けます。処理や決定の前に担当を取得します。取得は原子的で、他の担当者は閲覧だけが可能です。システム管理者は `force: true` でモデレーターの担当を引き継げますが、解放していない別のシステム管理者からは奪えません。

エスカレーションは担当を解放し、次の取得をシステム管理者に限定します。管理者から別の管理者へも送れます。各チケットは最大 3 回までで、3 回目の後の担当者は完了する必要があり、解放、追加エスカレーション、強制引き継ぎは無効です。チーム承認後は次のリポジトリ段階のため担当を解放します。

サポートの `process` は最終通知なしで処理済みにし、`complete` は完了、`close` は終了にします。これらは 1–4096 文字の `response` が必要です。フィードバックと提案は `outcome: resolved`、通報は `upheld` または `dismissed`、終了は `closed` を記録します。操作 JSON は最大 24 KiB。公開と移管は担当取得後に下記の決定経路を使います。

GET /api/tickets/{id} は閲覧可能な詳細と、現在の権限・担当から計算した `actions` 配列を返します。操作ボタンにこの配列を使用してください。担当の競合は `409` と `ticket_claim_required`、`ticket_occupied`、`ticket_escalation_limit` のいずれかを返します。

## 移管規則

申請者には、プロジェクトまたは公開ドメインに対する有効な L4 所有権、または現在のリポジトリ/システム
管理権限が必要です。グローバルチームへ移管する場合は、そのチームのメンバーでもなければなりません。
対象チームの T3/T4 管理者またはシステム管理者が決定します。申請者自身に審査権限があれば処理できます。

移管で変わるのは所有チームの関連付けだけです。パッケージ固有のメンバーは複製も削除もされません。
チーム間の直接移管はできないため、対象をいったん個人所有へ戻してから別の移管を申請します。

名前空間付き Docker イメージとスコープ付き npm パッケージはチームの不変プレフィックスを予約する
ため、個人所有へ戻せません。ミラー由来のリソースも移管できません。

## 公開規則

Maven リポジトリでは、審査なし、新規アーティファクトの初回バージョンのみ、すべてのバージョンのいずれかを
選択できます。審査を有効にすると再デプロイは無効になります。ローカルファイルは保存されますが、リポジトリ
モデレーターまたはシステム管理者が決定するまで公開インデックスには現れません。ミラーは対象外です。

分離 GPG 署名が必須の場合は、署名検証後に公開審査へ進みます。同じバージョンのチェックサム、署名、Maven
メタデータは一つのタスクに集約されます。最後のファイルから 5 秒間は決定できないため、アップロード途中で
処理されません。承認済みバージョンへのファイル追加も拒否されます。

npm の T2 member は、名前を予約せずに T3/T4 の team 承認から開始します。repository の作成審査も有効な場合は
同じ task が moderator 段階へ進み、無効なら package を原子的に作成します。最終承認では repository 権限と現在の
team membership を再確認して申請者へ L4 を付与します。`new_packages` では後続 version は通常どおり
公開されます。`every_version` では各 tarball と上限付き manifest/dist-tag payload も承認まで非公開にします。

Cargo 公開では crate archive を保存して非公開にし、sparse index と公開 catalog は変更しません。承認時は
archive を公開する前に、不変 version を両方の metadata に追加します。拒否時は非公開 archive を削除します。
`new_packages` では最初の公開 version が承認されるまで新規 crate として扱います。mirror 由来の crate は
審査対象外です。

Docker の T2 作成も npm と同じ順序で team 段階と任意の repository 段階を通ります。最終承認時に local/upstream
競合、repository 権限、現在の team membership を再確認して image を予約します。`new_packages` では後続 manifest
は通常どおり公開されます。
`every_version` では正確な manifest bytes を上限付き virtual file として保持し、承認前は reference と tag を
catalog に書き込みません。同じ digest の既存 tag は影響を受けず、manifest、blob link、tag、decision は原子的に
記録されます。mirror import は審査対象外です。

## タスク一覧

GET /api/tickets は上限付きページを返します。`view` は `reviewer` または `requested`、`status` は `unprocessed`（既定）、`in_progress`、`processed`、`closed`、`completed`、`all` です。`limit` は 1–100、`offset` は 0 以上です。カンマ区切りの `types` は既存処理の形式と `support`、`user`、`superteam`、`maven-domain`、`maven`、`cargo`、`npm`、`docker` に対応します。

`ticket_status` は共通の進行状態です。既存の `status` は処理結果の `pending`、`approved`、`rejected`、`cancelled` を維持します。過去の記録は最初の担当取得時にチケット状態を保存します。

応答には `tasks`、`total`、`limit`、`offset`、確定した `view` が含まれます。各タスクは移管元、移管先、現在の
審査チームのプレフィックス、申請者名、時刻、状態、決定情報を保持します。`review_team_prefix` が空でなければ
その team の T3/T4 が担当し、T2 作成の team 承認後はこの値を消して `pending` のまま repository 審査へ移ります。
公開タスクには `resource_version`、`file_count`、`total_size`、最新ファイル時刻も含まれます。
npm/Docker の明示的な作成では `resource_version` に予約値 `@create` を使用し、同じ file API から上限付き JSON
request を取得できます。

## 移管申請

POST /api/tickets/super-team-transfers は `resource_type`、`repository`、`resource_key`、
`target_team_prefix` を受け付けます。Maven 公開ドメインでは `repository` を省略し、Maven
アーティファクトでは `groupId:artifactId` をリソースキーにします。空の移管先は個人所有への復帰です。

同じリソースで保留にできる所有権移管は、移管先にかかわらず一つだけです。作成時は `201 Created`、タスク
本文、API の位置を返します。

## 審査ファイル

GET /api/tickets/{id}/files は、安定した識別子、サイズ、アップロード時刻、重要ファイル表示を持つ
リポジトリ相対パスを最大 256 件返します。GET /api/tickets/{id}/files/{file_id} は非公開ファイルを
ストリーミングします。申請者、現在割り当てられた team の T3/T4、担当 repository moderator、
system administrator のブラウザーセッションだけが利用できます。

Web のレビューセンターは最大 4 ワーカーで適応的にダウンロードし、失敗ごとに 2 回再試行します。すべて成功した
場合は標準リポジトリパスの ZIP をブラウザー内で作成します。失敗が残る場合は、不完全な ZIP の代わりに重要な
ファイルを個別に開きます。

## 決定または取消

現在の担当者がこの段階のチケットを取得済みである必要があります。POST /api/tickets/{id}/decision は `approved` または `rejected` を受け付けます。T2 package 作成の承認は作成を
完了するか、repository 審査が必要な場合に同じ task を空の `review_team_prefix` と `pending` 状態で返します。
移管の拒否には 512 文字以内の
理由が必要です。公開の拒否には `reason_code` として `invalid_metadata`、`quality`、`policy_violation`、
`copyright`、`malware`、`custom` のいずれかが必要です。独自理由は 505 文字までです。承認は対象エンジンの
バージョンメタデータを登録してからファイルを公開し、拒否は非公開ファイルを削除します。

DELETE /api/tickets/{id} は申請者による保留中のサポート、所有権移管、Maven 復元申請の撤回を認めます。チケットを終了し、以後の決定を阻止します。公開タスクはこの経路では取消できません。競合する最終決定はリソースを二重変更できません。

## エラー処理

失敗時は安定した `X-Renop-Error-Code` を返します。`400` は無効な絞り込み、リソース識別子、決定です。
`403` は所有権、対象チームのメンバー資格、レビュー権限の不足です。`404` はタスクまたはファイルが存在しない
場合、`409` は重複申請、決定済みタスク、所有権変更、禁止された移管、またはファイル受信中の公開を表します。

クライアントは登録済みコードを翻訳し、応答本文を直接表示してはいけません。

## 再取得した Maven パッケージの回復

`POST /api/tickets/maven-restorations` は現在の公開ドメインの L4 所有者の有効な `renop_session` Cookie を必要とします。

```json
{"resource_type":"maven_artifact","repository":"releases","resource_key":"com.example:demo"}
```

HTTP `201` と状態 `pending` の `maven_restore` タスクを返します。同じ保留申請があれば `409` を返します。
対象リポジトリのモデレーターとシステム管理者が閲覧でき、既存の決定・取消 API を使用します。
承認は現在のドメイン申請、有効な所有権、独立したロックをトランザクション内で再確認し、公開権限と現在のドメインチームへの関連付けを回復します。
申請状態が変化した場合は古い要求を取り消します。却下と取消は制限とダウンロードを維持します。
保留中の要求はアカウントごとに 64 件、全体で 4096 件までです。この種類にはダウンロード可能なレビュー用アーカイブはありません。
