---
title: クイックスタート
order: 3
category: はじめに
description: 初回起動とデフォルトリポジトリ URL
---

# クイックスタート

## 初回起動

初回起動時に RenoP は `admin` アカウントを作成します。サーバー起動前にパスワードを設定してください。

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

変数を設定しない場合、ランダムなパスワードがサーバーログに出力されます。起動後に `http://localhost:3000` を開いてください。

## デフォルトリポジトリ

| パス                              | 役割                   |
|-----------------------------------|------------------------|
| `http://localhost:3000/releases`  | リリース成果物         |
| `http://localhost:3000/snapshots` | スナップショット成果物 |
| `http://localhost:3000/private`   | プライベート成果物     |

Maven の `<repositories>` または `<distributionManagement>` にこれらの URL
のいずれかを指定します。詳細は [Maven クライアント設定](./maven-client.md) を参照してください。

## 環境変数

| 変数                           | デフォルト          | 用途                                         |
|--------------------------------|---------------------|----------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | サーバー、フロントエンド、ストレージ設定     |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | リポジトリ、ミラー、リポジトリ単位の S3 設定 |
| `RENOP_TOKENS`                 | `tokens.yaml`       | アカウントとアクセストークン                 |
| `RENOP_INDEX`                  | `index.json`        | 永続化された成果物インデックス               |
| `RENOP_SESSIONS`               | `sessions.json`     | 永続化されたログインセッション               |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 生成                | 最初の admin アカウントのパスワード          |

多くの設定は管理 UI からも変更できます。リスナーや TLS 設定を変更した場合はサーバーを再起動してください。
