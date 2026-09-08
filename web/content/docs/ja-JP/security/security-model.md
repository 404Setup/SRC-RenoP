---
title: セキュリティと権限
order: 1
category: セキュリティ
description: Credential boundary、repository permission、package team、defense in depth
---

# セキュリティと権限

RenoP は credential type、API Token capability、account role、repository visibility、対象 team を組み合わせて
認可します。所有 account が失った権限を credential が保持することはありません。

## Account / system role

| Role または permission                 | 効果                                                                      |
|:---------------------------------------|:--------------------------------------------------------------------------|
| Anonymous                              | `PUBLIC` と `HIDDEN` の既知 exact path を読む                             |
| `base`                                 | 暗黙 repository write のない認証 account                                  |
| `canview:{repo}` / `canview:*`         | 指定または全 repository を private 含め read                              |
| `canmoderate:{repo}` / `canmoderate:*` | 指定または全 repository の保留中 content を審査                           |
| `canupdate:{repo}` / `canupdate:*`     | package/domain policy の範囲で publish                                    |
| `showing`                              | 旧バージョンとの互換用権限。非表示リポジトリを一覧に表示する              |
| `allview` / `proview`                  | legacy global private-read alias                                          |
| `manager` / `admin`                    | user、repository、settings、audit、update、全 team の super administrator |

system admin は global です。package team L0-L4 は通常 collaboration の権限として分離します。admin operation
は audit され、表示 team member を暗黙作成しません。
moderator permission は審査に必要な private visibility を含みますが、publish、user 管理、repository 設定、
system settings の変更は許可しません。

管理者とモデレーターは、その権限を持つ間は利用停止にできません。利用停止にする前に、管理者とモデレーターの権限をすべて解除してください。利用停止中のアカウントにこれらの権限を付与するには、先に利用停止を解除する必要があります。データベースは権限変更や名前変更を含め、アカウントのトランザクション内で両方の操作を検証します。競合時は `409` と `ACCOUNT_BAN_PROTECTED` を返し、利用停止が拒否された場合は既存のセッションを維持します。

## アカウントと IP の利用停止

システム管理者はユーザーページで理由、有効期限（任意）を指定し、「記録済みのログイン IP もブロック」を選択してアカウントを利用停止にできます。サーバーは保持中のセッション（最終活動時刻が新しい64件）と過去30日間の直近256件のログイン成功記録から、最大64個の正規化したアドレスを収集します。有効な IP 制限に含まれるアドレスも保持します。ブラウザーから任意のアドレスを指定することはできません。利用できるアドレスがない場合、操作全体が `409 ACCOUNT_BAN_IP_UNKNOWN` で失敗します。オプションを外すとアカウントのみ利用停止にできます。

IP 制限、アカウントの利用停止、セッションの失効は同じトランザクションで確定します。制限は再起動や名前変更後も維持され、アカウントの利用停止と同時に期限が切れます。起動時には削除済みアカウントの恒久的な予約記録を維持し、認証情報やメンバーシップを再作成しません。チェックを外すとアカウントの利用停止を維持したまま IP 制限だけを削除し、利用停止を解除すると両方を削除します。同じアドレスが他のアカウントの有効な制限にも含まれる場合、ブロックは継続します。編集時に `ban_ip` を省略すると現在有効な IP 制限を維持します。

ブロックされたアドレスからの HTTP リクエストには、ログイン、登録、認証済みリクエスト、公開ダウンロードを含め、`403` と `IP_BANNED` を返します。同じアドレスを使う他のユーザーも影響を受けます。転送されたクライアントアドレスは、設定済みの信頼できるプロキシからのみ受け入れます。管理者向けの状態応答には保存済みアドレスそのものではなく件数を含めます。すべての利用停止 API はシステム管理者の権限を必要とし、非公開でキャッシュ不可のメタデータを返します。API トークンには `admin:users` スコープも必要です。

| メソッド | パス | リクエストまたは結果 |
|---|---|---|
| GET | `/api/tokens/:name/ban` | `{ban,ip_count,protected_role}` |
| PUT | `/api/tokens/:name/ban` | `{reason,expires_at,ban_ip}`。有効期限は null 可 |
| DELETE | `/api/tokens/:name/ban` | アカウントの利用停止と IP 制限を解除。`204` |

## Repository / team layer

- 可視性による発見と読み取りの境界は `PUBLIC`、権限に応じて表示する `HIDDEN`、認証が必要な `PRIVATE` です。
- repository permission は npm/Cargo/Docker package 作成や Maven domain 検証を自動で行いません。
- npm/Cargo/Docker team は L0 read、L1 publish、L2 lifecycle/metadata、L3 member、L4 owner です。
- Maven team は検証済み global domain に属し、全 Maven repository で有効です。
- private Docker image は public L0 を暗黙付与せず、blob も読める image に制限します。
- private npm package は scoped 名が必須で、明示 member または administrator を要求します。

## Credential transport

- **Browser session**: HttpOnly `renop_session`。private security と Token management に必要。
- **Basic**: username + password/API Token。標準 package protocol 専用。
- **Bearer API Token**: API/package automation の capability と exact target policy。
- **Docker Bearer**: source credential と image が許可した action だけの短期 token。

`Authorization: Session`、URL session secret、query credential は拒否します。Token scope/target は現在の
account authorization と常に交差します。

## Defense in depth

- password/recovery code は salted one-way verification、API Token plaintext は非永続です。
- session は idle expiry と device revoke に対応し、recovery は既存 session を atomic に失効します。
- rate limit、progressive ban、active bound、trusted proxy validation が network を保護します。
- upload、archive、mirror、update は bounded streaming、path validation、hash、temp storage を使います。
- audit と durable message は security result を記録し、neutral notification では operator を公開しません。
