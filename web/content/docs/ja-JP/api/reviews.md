---
title: レビュー API
order: 13
category: API リファレンス
description: 所有権移管とリポジトリ公開審査の独立処理
---

# レビュー API

レビュー API は所有権移管と審査対象の公開をメッセージセンターから分離します。タスクは永続化およびページ分割され、
決定は一度だけ適用されます。Docker イメージ、npm パッケージ、Cargo crate、Maven アーティファクト、
Maven 公開ドメインが対象です。

## 対象と認証情報

レビュー経路は認証済みブラウザーセッションだけを受け付けます。Basic 認証情報と Bearer API Token
では、タスクの作成、一覧、決定、取消を実行できません。アカウントメニューの `/account/reviews` から
同じ処理を利用できます。

レビュー担当ビューには、アカウントが T3/T4 であるチームの移管と T2 package 作成申請が表示されます。
必要なチーム段階が完了すると、担当リポジトリの公開審査がモデレーターへ表示されます。システム管理者はすべてのタスクを確認できます。申請者ビューは不変のアカウント
ID を使うため、ユーザー名変更前の申請も維持されます。

新しいタスクは現在の段階を担当する審査者とシステム管理者へ重複しないメッセージを送ります。T2 作成申請を
リポジトリ段階へ進めると、チーム通知を削除してモデレーター通知へ置き換えます。この時点では申請者へ承認結果を
送りません。最終決定後に残りの通知を削除し、申請者へローカライズ済み結果を送ります。申請者とシステム管理者
ではないモデレーターには `decided_by` を返しません。

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
競合、repository 権限、現在の team membership を再確認して image を予約します。`new_packages` では後続 manifest は通常どおり公開されます。
`every_version` では正確な manifest bytes を上限付き virtual file として保持し、承認前は reference と tag を
catalog に書き込みません。同じ digest の既存 tag は影響を受けず、manifest、blob link、tag、decision は原子的に
記録されます。mirror import は審査対象外です。

## タスク一覧

GET /api/reviews は上限付きページを返します。`view` は `reviewer` または `requested`、`status` は
`pending`、`approved`、`rejected`、`cancelled`、`all` を受け付けます。任意の `types` はカンマ区切りで
5 種類のリソースを指定します。`limit` は 1 から 100、`offset` は 0 以上です。

応答には `tasks`、`total`、`limit`、`offset`、確定した `view` が含まれます。各タスクは移管元、移管先、現在の
審査チームのプレフィックス、申請者名、時刻、状態、決定情報を保持します。`review_team_prefix` が空でなければ
その team の T3/T4 が担当し、T2 作成の team 承認後はこの値を消して `pending` のまま repository 審査へ移ります。
公開タスクには `resource_version`、`file_count`、`total_size`、最新ファイル時刻も含まれます。
npm/Docker の明示的な作成では `resource_version` に予約値 `@create` を使用し、同じ file API から上限付き JSON
request を取得できます。

## 移管申請

POST /api/reviews/super-team-transfers は `resource_type`、`repository`、`resource_key`、
`target_team_prefix` を受け付けます。Maven 公開ドメインでは `repository` を省略し、Maven
アーティファクトでは `groupId:artifactId` をリソースキーにします。空の移管先は個人所有への復帰です。

同じリソースで保留にできる所有権移管は、移管先にかかわらず一つだけです。作成時は `201 Created`、タスク
本文、API の位置を返します。

## 審査ファイル

GET /api/reviews/{id}/files は、安定した識別子、サイズ、アップロード時刻、重要ファイル表示を持つ
リポジトリ相対パスを最大 256 件返します。GET /api/reviews/{id}/files/{file_id} は非公開ファイルを
ストリーミングします。申請者、現在割り当てられた team の T3/T4、その段階後の担当 repository moderator、
system administrator のブラウザーセッションだけが利用できます。

Web のレビューセンターは最大 4 ワーカーで適応的にダウンロードし、失敗ごとに 2 回再試行します。すべて成功した
場合は標準リポジトリパスの ZIP をブラウザー内で作成します。失敗が残る場合は、不完全な ZIP の代わりに重要な
ファイルを個別に開きます。

## 決定または取消

POST /api/reviews/{id}/decision は `approved` または `rejected` を受け付けます。T2 package 作成の承認は作成を
完了するか、repository 審査が必要な場合に同じ task を空の `review_team_prefix` と `pending` 状態で返します。
移管の拒否には 512 文字以内の
理由が必要です。公開の拒否には `reason_code` として `invalid_metadata`、`quality`、`policy_violation`、
`copyright`、`malware`、`custom` のいずれかが必要です。独自理由は 505 文字までです。承認は対象エンジンの
バージョンメタデータを登録してからファイルを公開し、拒否は非公開ファイルを削除します。

DELETE /api/reviews/{id} では申請者だけが保留中の所有権移管を取り消せます。公開審査はこの経路では取り消せません。
保留状態に対する比較更新により、競合後の処理はリソースを変更せず競合応答になります。

## エラー処理

失敗時は安定した `X-Renop-Error-Code` を返します。`400` は無効な絞り込み、リソース識別子、決定です。
`403` は所有権、対象チームのメンバー資格、レビュー権限の不足です。`404` はタスクまたはファイルが存在しない
場合、`409` は重複申請、決定済みタスク、所有権変更、禁止された移管、またはファイル受信中の公開を表します。

クライアントは登録済みコードを翻訳し、応答本文を直接表示してはいけません。
