---
title: 設定の概要
order: 1
category: 設定
description: 設定ファイル、サーバー設定、環境変数
---

# 設定の概要

設定と状態はプロセスの作業ディレクトリ。パスは環境変数で上書き可。

## ファイル

| ファイル            | 環境変数上書き       | 用途                                                  |
|---------------------|----------------------|-------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | バインド、TLS、フロントブランド、ストレージパス、更新 |
| `repositories.yaml` | `RENOP_REPOSITORIES` | リポジトリ、ミラー、リポジトリ単位の S3               |
| `tokens.yaml`       | `RENOP_TOKENS`       | ユーザー、ロール、アップロードトークン                |
| `index.json`        | `RENOP_INDEX`        | 成果物インデックスキャッシュ                          |
| `sessions.json`     | `RENOP_SESSIONS`     | ブラウザログインセッション                            |

実行時に関連:

| 変数                           | 既定     | 用途                                  |
|--------------------------------|----------|---------------------------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自動生成 | 最初の `admin` アカウントのパスワード |

## `config.yaml` の構造

### `storage_path`

ローカル成果物ストレージのルートディレクトリ（このパス配下の既定レイアウト）。相対パスの既定は通常 `storage` です。

### `server`

| キー                  | 既定              | 説明                                                                   |
|-----------------------|-------------------|------------------------------------------------------------------------|
| `host`                | `0.0.0.0`         | 待ち受けアドレス                                                       |
| `port`                | `3000`            | 待ち受けポート                                                         |
| `ssl_enabled`         | `false`           | TLS を有効化                                                           |
| `ssl_cert_path`       | `""`              | TLS 有効時の証明書パス                                                 |
| `ssl_key_path`        | `""`              | TLS 有効時の秘密鍵パス                                                 |
| `domains`             | `[localhost]`     | 公開ホスト名（UI / メタデータ + 既定 CORS）                            |
| `cors_origins`        | `[]`              | ブラウザ CORS 許可リスト（空 = `domains` のみ、`*` = すべて）          |
| `enable_compression`  | `false`           | HTTP 応答圧縮                                                          |
| `file_cache_size_mb`  | `100`             | メモリ上のファイルキャッシュサイズ（MB）                               |
| `max_active_requests` | `2000`            | 同時リクエスト上限（超過時 503）                                       |
| `trusted_proxies`     | `[]`              | 追加のリバースプロキシ CIDR/IP（ループバックは常に信頼）               |
| `cdn_ip_header`       | `X-Forwarded-For` | 信頼プロキシ背後でのクライアント IP ヘッダー（例: `CF-Connecting-IP`） |

host、port、TLS を変更したらプロセスを **再起動** してください。

### `frontend`

埋め込みリポジトリブラウザのブランド設定:

| キー                   | 説明                              |
|------------------------|-----------------------------------|
| `id`                   | フロントエンド / サイト ID        |
| `title`                | ページタイトル                    |
| `description`          | 短い説明                          |
| `organization_website` | 組織 / 製品 URL                   |
| `organization_logo`    | ロゴパス（例: `/svg/logo.svg`）   |
| `background_url`       | 任意の背景画像                    |
| `icp_license`          | 任意の ICP / コンプライアンス文言 |

### `updater`

| キー      | 既定      | 説明                              |
|-----------|-----------|-----------------------------------|
| `channel` | `release` | `release` または `nightly`        |
| `mode`    | `manual`  | 更新の適用方法（例: UI から手動） |

サイトの[ダウンロード](/download) は同じ stable / nightly ソース。

## 管理 UI

**manager** / **admin** は設定・リポジトリで大半を変更可。ファイル変更は再読込 / 再起動が要る場合あり。

## ストレージ

- **ローカルディスク**（`storage_path`、既定）
- **S3 互換**（`repositories.yaml` でリポジトリ単位）

アップロード時に MD5 / SHA-1 / SHA-256 / SHA-512 サイドカーを書ける。

可視性・ミラー・S3: [リポジトリとミラー](./repositories.md)。

## 関連

- [クイックスタート](../getting-started/quickstart.md)
- [Maven クライアント](../getting-started/maven-client.md)
- [API 索引](../api/README.md)
