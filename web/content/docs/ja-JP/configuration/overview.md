---
title: 設定の概要
order: 1
category: 設定
description: 設定ファイル、サーバー、ストレージ、プロキシ、ブランド、更新
---

# 設定の概要

RenoP は作業ディレクトリの `config.yaml` を読み、`RENOP_CONFIG` で上書きできます。管理 UI からの書き込みも
同じ検証済み構造と非公開ファイル権限を使用します。

## 設定ファイル

| ファイル | 上書き | 用途 |
|:---------|:-------|:-----|
| `config.yaml` | `RENOP_CONFIG` | サーバー、DB、preview、proxy、frontend、audit、updater |
| `repositories.yaml` | `RENOP_REPOSITORIES` | engine、visibility、mirror、Maven policy、S3 |
| `index.json` | `RENOP_INDEX` | ストレージから再構築できるファイル索引 snapshot |

アカウント、API Token、session、team、audit、message は DB に保存し、YAML では設定しません。資格情報を含む
場合があるため、設定ファイルはサービスアカウントだけが読めるようにします。

## `config.yaml` スキーマ

### ストレージとドキュメント preview

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
enable_cargodoc_preview: true
cargodoc_extract_path: ""
max_cargodoc_size_mb: 128
```

抽出先が空なら platform cache を使います。`/javadoc` または `/cargodoc` で公開する前に path と size を
検証します。

### `server` の network と security

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains: ["localhost"]
  cors_origins: []
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: []
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
  gpg:
    key_servers: ["https://keys.openpgp.org", "https://keyserver.ubuntu.com"]
```

`domains` は公開 host と既定 CORS host です。`cors_origins` は exact origin、host、wildcard を追加し、`*` は
すべてを許可します。転送 IP header は接続元が `trusted_proxies` に一致する場合だけ信頼します。host、port、
TLS、compression、debug、一部 cache の変更は再起動が必要です。

GitHub OAuth は `server.github_oauth` に保存し、Client ID と書き込み専用 Secret は UI で設定します。

### `database` 接続

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

`sqlite3`（または `sqlite`）、`mysql`、`postgres`、ネイティブ `clickhouse` に対応します。
[データベース設定](./database.md)を参照してください。

### `proxy` 送信ルート

```yaml
proxy:
  selected: ""
  proxies:
    - name: "corp_proxy"
      url: "http://proxy.internal:8080"
      username: ""
      password: ""
```

HTTP、HTTPS、SOCKS5 proxy を最大 16 件設定できます。[送信プロキシ](./outbound-proxy.md)を参照してください。

### `frontend` ブランド

```yaml
frontend:
  id: "renop"
  title: "RenoP Package Registry"
  description: "Self-hosted package repository"
  organization_website: ""
  organization_logo: "/svg/logo.svg"
  background_url: ""
  font_preset: "system"
  font_url: ""
  icp_license: ""
  public_security_filing: ""
  legal_notice_url: ""
```

URL は使用前に検証します。背景画像は WebP と size policy を満たす必要があります。
`font_preset` には `system`、`inter`、`noto_sans`、`open_sans`、`source_sans`、`custom` を指定できます。
プリセットはローカルにインストールされたフォントを使用します。カスタムフォントは同一オリジンのパスまたは HTTP(S) URL から
バックグラウンドで取得し、完全に読み込まれた後でのみ有効になるため、初回描画を妨げません。

### `updater` 方針

```yaml
updater:
  channel: "release"
  mode: "manual"
```

`channel` は `release` または `nightly`、`mode` は `manual`、`auto_check`、`auto_install` です。自動確認は
process scheduler が統合し、結果を管理者へ通知します。
