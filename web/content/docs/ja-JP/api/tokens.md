---
title: API Token とユーザー
order: 3
category: API リファレンス
description: 細粒度 API Token のライフサイクル、認証境界、ユーザー管理
---

# API Token とユーザー

API Token は 1 つのアカウントが所有する永続的な機械資格情報です。RenoP は 256 bit のランダム秘密値に
対する SHA-256 検索 digest だけを保存します。平文は作成時に一度だけ返り、後から復元できません。

各要求は独立した 2 つの条件を満たす必要があります。

- Token が API に必要な能力を持つこと。
- 所有アカウントが対象リソースを現在も操作できること。

したがってロール、リポジトリ権限、チーム所属の変更は Token を作り直さずに反映されます。

## API Token の管理

管理ルートには HttpOnly `renop_session` Cookie が必要です。API Token、パスワード、
`Authorization: Session`、URL クエリでは秘密値を管理できません。

### 割り当て可能 scope の一覧

`GET /api/auth/profile/api-tokens/scopes`

レスポンスは現在のアカウント権限で絞られ、一般アカウントに管理者 scope は提示されません。

```json
{
  "scopes": ["repository:read", "repository:publish", "package:metadata"],
  "target_kinds": {
    "repository:read": "repository",
    "repository:publish": "repository",
    "package:metadata": "package"
  },
  "target_limit": 128
}
```

### Token の作成

`POST /api/auth/profile/api-tokens`

```json
{
  "name": "CI publishing",
  "scopes": ["repository:read", "repository:publish"],
  "targets": {
    "repository:publish": ["releases"]
  },
  "expires_at": 1798761600000
}
```

`expires_at` は作成後 5 分～5 年の Unix millisecond で、省略または null は Token 単位の期限なしです。
1 アカウントは最大 50 Token を所有できます。

`targets` は scope ごとに独立して対象を制限します。`targets` にない scope は、アカウントが現在許可された
すべての対象で使えます。リポジトリ対象は正確な名前、パッケージ対象は `repository/package` です。Maven
では `maven-releases/com.example/library` のように指定します。チーム対象は
`package/repository/package` または `domain/example.com`、ドメイン対象は正規ドメイン名です。合計上限は
128 対象です。

対象制限がリポジトリ権限や現在の L0-L4 を迂回することはありません。

成功時は `201 Created` と `Cache-Control: no-store` を返します。

```json
{
  "token": {
    "id": "07cdcf2e-0828-4a29-9817-cf771cc9fb0a",
    "name": "CI publishing",
    "scopes": ["repository:publish", "repository:read"],
    "targets": {"repository:publish": ["releases"]},
    "created_at": 1787731200000,
    "expires_at": 1798761600000
  },
  "secret": "rnp_pat_EXAMPLE_REDACTED_COPY_THE_REAL_VALUE_ONCE"
}
```

### Token メタデータの一覧

`GET /api/auth/profile/api-tokens` は秘密値を含まないメタデータとアカウント上限だけを返します。

### Token の失効

`DELETE /api/auth/profile/api-tokens/{token_id}` は `204 No Content` を返し、認証キャッシュを直ちに無効化します。

## scope リファレンス

| Scope                 | 能力                                                            |
|:----------------------|:----------------------------------------------------------------|
| `repository:read`     | カタログ、メタデータ、ファイル、イメージ、バージョンの読み取り  |
| `repository:publish`  | Maven、npm、Cargo、Docker、files、分割アップロードでの公開      |
| `repository:delete`   | ファイル、バージョン、タグ、イメージの削除                      |
| `package:create`      | リポジトリ認可後の npm/Cargo package または Docker image の予約 |
| `package:metadata`    | パッケージ説明とメタデータの更新                                |
| `package:lifecycle`   | package/version の archive、restore、yank、unyank               |
| `team:manage`         | npm、Cargo、Docker、Maven domain のチームと招待の閲覧・管理     |
| `domain:read`         | 非公開 Maven domain 設定の読み取り                              |
| `domain:create`       | Maven domain の作成                                             |
| `domain:verify`       | Maven domain の検証または強制検証                               |
| `domain:delete`       | Maven domain の削除                                             |
| `messages:read`       | アカウントメッセージの閲覧、既読化、削除                        |
| `account:read`        | 非公開アカウント情報と個人監査ログの読み取り                    |
| `account:write`       | API による公開プロフィール更新                                  |
| `statistics:read`     | アカウントが閲覧できるダウンロード統計の照会                    |
| `admin:users`         | ユーザーとログインデバイスの管理                                |
| `admin:repositories`  | リポジトリ管理とインデックス再構築                              |
| `admin:settings`      | システム設定と診断の管理                                        |
| `admin:audit`         | 管理者向け監査・状態データの読み取りと消去                      |
| `admin:notifications` | 管理者通知の作成                                                |
| `admin:updates`       | 更新の確認、アップロード、インストール、再起動                  |
| `admin:statistics`    | システム全体の統計照会                                          |

`admin:*` は管理者だけが作成でき、所有アカウントが管理者でなくなると効力を失います。既存 Token の
`package:manage` と `domain:manage` は互換性のため受理しますが、新規割り当てはできません。

## Token の使用

許可された管理 API では Bearer を使用します。

```http
Authorization: Bearer rnp_pat_REDACTED
```

パッケージクライアントは同じ Token をユーザー名に対する Basic password として使えます。Basic は
パッケージプロトコル専用です。npm は `_authToken` または Basic、Cargo は Token 全体を `Authorization` として送ります。Docker
は
`/v2/token` で短期 Token に交換し、scope とイメージ権限の両方で許可された操作だけを含めます。

## 互換 API

管理者のユーザー CRUD は `/api/tokens` に残りますが、管理者が他ユーザーの資格情報を作ることはできません。
旧 `POST /api/auth/profile/token` はログイン中アカウントに期限なしの追加公開 Token を作ります。新しい統合は
細粒度プロフィール API を使用してください。
