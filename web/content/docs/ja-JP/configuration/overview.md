---
title: 設定の概要
order: 1
category: 設定
description: 設定ファイル、サーバー設定、環境変数
---

# 設定の概要

RenoP は設定と状態をプロセスの作業ディレクトリに保存します。パスは環境変数で上書きできます。

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

サイトの[ダウンロード](/download)ページは同じ release / nightly ソースを使います。

## 管理 UI

サインインした **manager** / **admin** は **設定** と **リポジトリ** で多くの項目を編集できます。ファイル単位の変更は
各ドメインの説明どおり、再読込 / 再起動後に反映されます。

## ストレージバックエンド

成果物は次に置けます:

- **`storage_path` 配下のローカルディスク**（既定）
- **S3 互換オブジェクトストレージ**（`repositories.yaml` でグローバルまたはリポジトリ単位）

アップロード時にチェックサムサイドカー（MD5 / SHA-1 / SHA-256 / SHA-512）を生成できます。

可視性・ミラー・S3 フィールドは [リポジトリとミラー](./repositories.md) を参照。

## 関連

- [クイックスタート](../getting-started/quickstart.md)
- [Maven クライアント](../getting-started/maven-client.md)
- [API 索引](../api/README.md)
