---
title: クイックスタート
order: 3
category: はじめに
description: 初回起動、管理者、health check、repository 作成
---

# クイックスタート

## サーバー起動

初回起動時、RenoP は DB に `admin` super administrator を作成します。パスワードを明示してください。

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

未設定ならランダムパスワードを生成し stdout に一度だけ表示します。直ちに保存して
`http://localhost:3000` を開きます。既定 bind は `0.0.0.0:3000` です。本番は TLS または trusted proxy を
使用してください。

## 既定と新規リポジトリ

初期 `repositories.yaml` には互換用 Maven repository が 3 件あります。

| Path         | Visibility | Policy                   |
|:-------------|:-----------|:-------------------------|
| `/releases`  | `PUBLIC`   | Maven、redeployment 無効 |
| `/snapshots` | `PUBLIC`   | Maven、redeployment 有効 |
| `/private`   | `PRIVATE`  | Maven、認証必須          |

npm、Cargo、Docker、`files` は管理画面から明示的に作成します。Docker image と npm package は各 repository
画面で予約後に push できます。Cargo name は上流検査後に作成します。Maven 公開には検証済み domain が必要です。

## health 確認

```bash
curl -s http://localhost:3000/api/status/health
# Output: "UP"
```

protobuf runtime metric は `/api/status/instance` です。health は process が応答することだけを示すため、本番
traffic 前に実際の認証操作で DB と storage も検証してください。

## 主要な環境変数

| 変数                           | 既定                | 用途                              |
|:-------------------------------|:--------------------|:----------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | main config path                  |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | repository config path            |
| `RENOP_INDEX`                  | `index.json`        | file-index snapshot path          |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 1 回生成            | `admin` がない場合の初期 password |

account、session、team、API Token、audit、message は DB data であり YAML path 変数はありません。

## 次の手順

- [設定概要](../configuration/overview.md) — TLS、DB、proxy、preview、updater
- [リポジトリとミラー](../configuration/repositories.md) — engine、visibility、upstream、migration、S3
- [Maven / Gradle](../guides/maven-client.md) — domain 検証と JVM client
- [Cargo Registry](../guides/cargo-registry.md) — repository 作成と crate 公開
- [Docker Registry](../guides/docker-registry.md) — push 前の image 作成と client 設定
- [npm Registry](../guides/npm-registry.md) — package 予約と npm 互換 client の設定
