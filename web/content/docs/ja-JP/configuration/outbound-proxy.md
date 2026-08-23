---
title: 送信プロキシ設定
order: 4
category: 設定
description: HTTP、HTTPS、SOCKS5 送信プロキシの設定とミラー別ルーティング
---

# 送信プロキシ設定

内部ネットワークから上流リポジトリにアクセスするためのプロキシ設定です。

```yaml
proxy:
  selected: "corp_http"   # デフォルトプロキシ名（空欄で直接接続）
  proxies:
    corp_http:
      url: "http://10.0.0.1:8080"
    socks_proxy:
      url: "socks5://user:pass@10.0.0.2:1080"
```

`repositories.yaml` 内のミラーごとに `proxy: "corp_http"` や `proxy: "direct"` を指定して個別にルーティングを制御できます。
