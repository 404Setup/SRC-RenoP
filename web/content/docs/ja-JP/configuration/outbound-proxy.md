---
title: 送信プロキシ
order: 4
category: 設定
description: 名前付き HTTP、HTTPS、SOCKS5 プロキシとミラー単位のルーティング
---

# 送信プロキシ設定

Maven Central、crates.io、Docker、GitHub、GitLab、GPG 鍵サーバーへ管理された経路で接続する場合に設定します。
プロセスは routing policy ごとに上限付き HTTP transport を共有します。

## 設定スキーマ

```yaml
proxy:
  selected: "corp_http"
  proxies:
    - name: "corp_http"
      url: "http://10.0.0.1:8080"
      username: "proxy-user"
      password: "proxy-password"
    - name: "socks_proxy"
      url: "socks5://10.0.0.2:1080"
      username: ""
      password: ""
```

`selected` は全体の既定値で、空なら直接接続です。名前付きプロキシは最大 16 件で、名前は一意です。URL は
`http`、`https`、`socks5` を使用し、適切な host/port を含めます。資格情報、path、query、fragment を URL
に含めず、`username` と `password` に分離してください。

## ルーティング動作

| セレクター | 結果 |
|:-----------|:-----|
| `""` | `proxy.selected` を継承 |
| `direct` | すべてのプロキシを迂回 |
| プロキシ名 | 指定した設定を使用 |

選択または資格情報の変更時は関連する共有 client を無効化します。不明な名前は直接接続へ fall back せず
拒否します。

## ミラー単位の選択

各ミラーは `proxy` で全体設定を上書きできます。

```yaml
repositories:
  releases:
    name: releases
    format: maven
    mirrors:
      - name: "maven-central"
        url: "https://repo1.maven.org/maven2"
        proxy: "corp_http"
      - name: "internal"
        url: "https://mirror.internal/maven"
        proxy: "direct"
      - name: "default-route"
        url: "https://plugins.gradle.org/m2"
        proxy: ""
```

グローバルプロキシを通してはならない内部サービスには `direct` を使います。ミラー URL やログに秘密値を
含めないでください。
