---
title: 設定
order: 6
category: API
---

# 設定とリポジトリ構成

プレフィックス: `/api/settings`

読み書きには **manager / admin** が必要です。

このプレフィックス下で構造化データを運ぶリクエスト / レスポンス本文はすべて **`application/x-protobuf`** です（
`proto/api/v1/api.proto` 参照）。空の成功本文は プレーンテキスト（`""`）のままです。検証エラーは短い英語テキストのままです。

ディスク上の場所:

| 内容             | ファイル            | 環境変数             |
|------------------|---------------------|----------------------|
| ドメイン設定     | `config.yaml`       | `RENOP_CONFIG`       |
| Maven リポジトリ | `repositories.yaml` | `RENOP_REPOSITORIES` |

リスナー / TLS の変更はプロセス再起動で完全に適用されます。

## インデックス

### `POST /api/settings/index/rebuild`

リクエスト: protobuf `RebuildIndexRequest`

| フィールド | 型     | 値               |
|------------|--------|------------------|
| `mode`     | string | `full` \| `diff` |

| mode   | 動作                                         |
|--------|----------------------------------------------|
| `full` | 非同期のフル再構築。Javadoc キャッシュを消去 |
| `diff` | 差分再構築                                   |

それ以外 → 400（`Invalid mode. Expected 'full' or 'diff'`）。成功: 200、空文字列本文。

## 設定ドメイン

### `GET /api/settings/domains`

レスポンス: protobuf `SettingsDomainsResponse`

| フィールド | 型              |
|------------|-----------------|
| `domains`  | repeated string |

典型値: `frontend`、`server`、`storage`、`updater`、`index`。

`index` には現在設定可能なフィールドはありません。

### `GET /api/settings/domain/:name`

レスポンス: そのドメインの protobuf メッセージ（Content-Type `application/x-protobuf`）。

**frontend** → `FrontendConfig`

| フィールド               | 型     |
|--------------------------|--------|
| `id`                     | string |
| `title`                  | string |
| `description`            | string |
| `organization_website`   | string |
| `organization_logo`      | string |
| `background_url`         | string |
| `icp_license`            | string |
| `public_security_filing` | string |
| `legal_notice_url`       | string |

**server** → `ServerConfig`

| フィールド            | 型              | 説明                                               |
|-----------------------|-----------------|----------------------------------------------------|
| `host`                | string          | 待ち受け IP アドレス                               |
| `port`                | uint32          | 待ち受けポート                                     |
| `ssl_enabled`         | bool            | TLS を有効にするか                                 |
| `ssl_cert_path`       | string          | TLS 証明書ファイルパス                             |
| `ssl_key_path`        | string          | TLS 秘密鍵ファイルパス                             |
| `domains`             | repeated string | このインスタンスの公開ホスト名リスト               |
| `enable_compression`  | bool            | HTTP 応答圧縮を有効にするか                        |
| `file_cache_size_mb`  | uint32          | メモリ上のファイルキャッシュ上限（MB）             |
| `max_active_requests` | uint32          | アクティブな同時リクエスト数上限                   |
| `trusted_proxies`     | repeated string | 信頼できるプロキシの CIDR/IP リスト                |
| `cdn_ip_header`       | string          | クライアント IP ヘッダー名                         |
| `cors_origins`        | repeated string | 許可する CORS オリジンリスト                       |
| `debug_mode`          | bool            | デバッグプロファイリング API を有効化するか        |
| `database`            | DatabaseConfig  | データベース接続設定                               |

**DatabaseConfig**:

| フィールド               | 型     | 説明                                               |
|--------------------------|--------|----------------------------------------------------|
| `enabled`                | bool   | データベースの永続化を有効にするか                 |
| `driver`                 | string | データベースドライバー（`sqlite3` または `mysql`）  |
| `dsn`                    | string | データベース DSN またはパス（例: `renop.db`）      |
| `max_open_conns`         | int32  | 最大オープン接続数                                 |
| `max_idle_conns`         | int32  | 最大アイドル接続数                                 |
| `conn_max_lifetime_sec`  | int32  | 接続の最大生存期間（秒）                           |

**storage** → `StorageConfig`

| フィールド               | 型     |
|--------------------------|--------|
| `storage_path`           | string |
| `enable_javadoc_preview` | bool   |
| `javadoc_extract_path`   | string |
| `max_javadoc_size_mb`    | int64  |

**updater** → `UpdaterConfig`

| フィールド | 型     | 値                                                           |
|------------|--------|--------------------------------------------------------------|
| `channel`  | string | `release` \| `nightly`                                       |
| `mode`     | string | `manual` \| `auto_check` \| `auto_install` \| `safe_install` |

**index** → 空の `IndexDomainSettings`

### `PUT /api/settings/domain/:name`

ドメインの **完全置換**。本文はそのドメインの GET と同じ protobuf メッセージ。 Proto3
の省略フィールドはゼロ値としてデコードされます — クライアントは完全なドメイン設定を送る必要があります （UI は常にフォーム全体の状態を
POST します）。

成功: 200、空文字列。

規則:

- `frontend.background_url`: 非空のとき到達可能で、公開 IP、WebP、≤ 5 MiB。プライベートアドレスは拒否
- `storage.max_javadoc_size_mb`: > 0 必須
- `storage.storage_path`: 別パスへ変更すると、サーバーは新しいルートのファイルインデックスを即座にフル再構築（FS
  ウォッチャーも再起動）。Javadoc キャッシュを消去
- `updater.channel` / `updater.mode`: 許可された列挙値のみ（空は無効）
- `index`: 書き込み可能項目なし → 404

検証失敗 → 400 + 短い英語エラーテキスト。

## Maven リポジトリ

### `GET /api/settings/maven/repositories`

レスポンス: protobuf `MavenRepositoriesResponse`（`map<string, Repository>`）。

| フィールド           | 意味                                                   |
|----------------------|--------------------------------------------------------|
| `name`               | リポジトリ名                                           |
| `visibility`         | `PUBLIC` / `HIDDEN` / `PRIVATE`                        |
| `allow_redeployment` | 既存成果物の上書き可否                                 |
| `mirrors[]`          | 上流ミラー（url、persist、TTL、auth、allow/deny など） |
| `s3`                 | 任意の S3 互換ストレージ                               |

### `PUT /api/settings/maven/repositories/:name`

作成または **完全置換**。本文は protobuf `Repository`。パスの `:name` が本文の `name` より優先。

予約名: `css`、`js`、`svg`、`api`、`javadocs`、`assets`、および不正な文字。

成功: 200、空文字列。

### `DELETE /api/settings/maven/repositories/:name`

設定から削除。ディスク上のファイルは **削除しません**。成功: 200、空文字列。
