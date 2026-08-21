---
title: API 一覧
order: 1
category: API
---

# RenoP HTTP API

デフォルトの待ち受けアドレス: `0.0.0.0:3000`。

| パス        | 用途                                                         |
|-------------|--------------------------------------------------------------|
| `/api/*`    | 管理 API（ログイン、設定、ステータスなど）                   |
| `/{repo}/…` | Maven リポジトリレイアウト（ダウンロード/アップロード/削除） |

エラー本文はしばしばプレーンテキスト（`Unauthorized`、`Forbidden`、`Not found`）です。まずステータスコードを信頼してください。

## 索引

| ファイル                                 | 内容                                                |
|------------------------------------------|-----------------------------------------------------|
| [authentication.md](./authentication.md) | ログイン、セッション、権限                          |
| [tokens.md](./tokens.md)                 | アカウント管理（manager）                           |
| [maven.md](./maven.md)                   | 閲覧、バージョン、バッジ、POM 生成                  |
| [gpg.md](./gpg.md)                       | GPG 鍵の登録、署名付きアップロード、検証            |
| [status.md](./status.md)                 | ヘルスとランタイムステータス                        |
| [settings.md](./settings.md)             | 設定ドメイン、リポジトリ、インデックス再構築        |
| [updater.md](./updater.md)               | オンライン/オフライン更新                           |
| [storage.md](./storage.md)               | リポジトリパス上の GET/PUT/DELETE、分割アップロード |
| [rate-limit.md](./rate-limit.md)         | IP レート制限、認証失敗バン、同時リクエスト上限     |

機械可読スキーマ: [openapi.yaml](/assets/openapi.yaml)。  
Proto 定義: `proto/api/v1/api.proto`（生成 Go コードは `pb/` 配下）。

## JSON と Protobuf

多くのエンドポイントはまだ JSON です。次は `application/x-protobuf` を使います。

| エンドポイント                               | 方向               |
|----------------------------------------------|--------------------|
| `POST /api/auth/login`                       | request + response |
| `GET /api/auth/me`                           | response           |
| `GET /api/tokens`                            | response           |
| `GET /api/status/instance`                   | response           |
| `GET /api/status/snapshots`                  | response           |
| `GET /api/updater/status`                    | response           |
| `POST /api/settings/index/rebuild`           | request            |
| `GET /api/settings/domains`                  | response           |
| `GET /api/settings/domain/:name`             | response           |
| `PUT /api/settings/domain/:name`             | request            |
| `GET /api/settings/maven/repositories`       | response           |
| `PUT /api/settings/maven/repositories/:name` | request            |
| `GET /api/maven/details…`                    | response           |
| `GET /api/maven/repo-details/:repo`          | response           |
| `GET /api/maven/signatures…`                 | response           |
| `GET /api/auth/profile/gpg`                  | response           |
| `POST /api/auth/profile/gpg`                 | request + response |
| `GET /api/auth/profile/gpg/releases`         | response           |
| `POST /api/upload/chunked/`                  | request + response |
| `POST /api/upload/chunked/:id/complete`      | response           |

フィールド名は proto に合わせます（snake_case）。`protoc` でクライアントを生成するか、フロントエンドの `protobufjs`
コーデックに従ってください。

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

```bash
# ログイン後、Cookie 名は renop_session
curl -s -b 'renop_session=<session-id>' \
  -H 'Accept: application/x-protobuf' \
  http://localhost:3000/api/auth/me \
  -o me.bin
```

## 認証

対応する搬送手段:

1. Cookie: `renop_session=<id>`
2. `Authorization: Session <id>`
3. `Authorization: Basic base64(user:password_or_upload_token)`
4. `Authorization: Bearer <user>:<secret>` または `Bearer <upload-token>`
5. GET/HEAD のみ: `?token=<session-or-bearer>`

セッションは約 **7 日間** のアイドルで期限切れし、活動時に更新されます。

| ロール          | 能力                                                   |
|-----------------|--------------------------------------------------------|
| 匿名            | PUBLIC リポジトリの読み取り。管理 API は多くが 401/403 |
| 一般ユーザー    | `canview:` / `canupdate:` 経由でリポジトリにアクセス   |
| manager / admin | ユーザー、設定、updater などの管理 API                 |

詳細: [authentication.md](./authentication.md)。

## ステータスコード

| コード | 意味                                                             |
|--------|------------------------------------------------------------------|
| 200    | OK（本文は空またはプレーンテキストの場合あり）                   |
| 201    | アップロード作成                                                 |
| 204    | 成功、本文なし                                                   |
| 400    | 不正なパラメータ / 本文                                          |
| 401    | 未認証または無効な資格情報                                       |
| 403    | 不許可、期限切れ、または繰り返しの 401/403 後の IP バン          |
| 404    | 不存在。プライベート読み取りは 403 の代わりに 404 を返す場合あり |
| 409    | 競合（名前取得済み、更新が実行中）                               |
| 429    | 匿名 IP がリクエストレート制限を超過                             |
| 503    | 過負荷（例: 同時リクエスト上限）                                 |
| 507    | ディスク容量不足                                                 |

レート制限と異常検知: [rate-limit.md](./rate-limit.md)。

インスタンス版: `GET /api/status/instance` の `version`。独立した API バージョン欄はありません。
