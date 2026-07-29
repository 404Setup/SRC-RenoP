---
title: クイックスタート
order: 3
category: はじめに
description: 初回起動、admin パスワード、デフォルトリポジトリ URL
---

# クイックスタート

## 初回起動

初回起動時に `admin` アカウントが作成されます。プロセス起動前に環境変数でパスワードを設定してください。

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

未設定の場合、ランダムなパスワードが生成され、サーバーログに出力されます。起動後は `http://localhost:3000` を開いてください。

`admin` でサインインします。manager または admin 権限を持つアカウントは、Web UI で成果物・ユーザー・リポジトリ・設定を管理できます。

## デフォルトリポジトリ

| パス                              | 用途      |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

これらの URL を Maven の `<repositories>` または `<distributionManagement>`
に設定します。例は [Maven クライアント](./maven-client.md) を参照してください。

## ヘルスチェック

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## 環境変数

| 変数                           | デフォルト          | 用途                                                            |
|--------------------------------|---------------------|-----------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | サーバー、フロントエンド、ストレージ、updater                   |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | リポジトリ、ミラー、リポジトリ単位の S3                         |
| `RENOP_TOKENS`                 | `tokens.yaml`       | アカウントとトークン                                            |
| `RENOP_INDEX`                  | `index.json`        | 成果物インデックス                                              |
| `RENOP_SESSIONS`               | `sessions.bin`      | ログインセッション（protobuf。旧 `sessions.json` は移行される） |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 生成                | 最初の admin アカウントのパスワード                             |

多くの設定は管理 UI からも変更できます。待ち受けアドレスまたは TLS を変更したあとはプロセスの再起動が必要です。

## 次の手順

1. [設定](../configuration/overview.md) — 待ち受け、TLS、ブランド
2. [リポジトリとミラー](../configuration/repositories.md)
3. [Maven クライアント](./maven-client.md)
4. [HTTP API](../api/README.md)
