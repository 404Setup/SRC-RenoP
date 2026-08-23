---
title: 設定概要
order: 1
category: 設定
description: config.yaml の設定パラメータ、サーバー設定、環境変数
---

# 設定概要

RenoP のメイン設定ファイルは `config.yaml` です。

## `config.yaml` の設定項目

### ストレージとドキュメントプレビュー

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
```

### サーバーネットワークとセキュリティ (`server`)

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains:
    - "localhost"
  cors_origins: []
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: []
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
```

> **注意**: `host`、`port`、TLS 関連の設定変更を反映するにはプロセスの再起動が必要です。

### データベース (`database`)

```yaml
database:
  enabled: true
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

### 送信プロキシ (`proxy`)

```yaml
proxy:
  selected: ""
  proxies:
    corp_proxy:
      url: "http://proxy.internal:8080"
```
