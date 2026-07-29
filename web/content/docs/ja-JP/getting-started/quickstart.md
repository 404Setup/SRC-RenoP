---
title: クイックスタート
order: 3
category: はじめに
description: 初回起動、admin パスワード、デフォルトリポジトリ URL
---

# クイックスタート

## 初回起動

初回起動で `admin` アカウントが作られる。起動前にパスワードを設定:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

未設定ならランダムパスワードがサーバーログに出る。その後 `http://localhost:3000` を開く。

`admin` でログイン。manager は Web UI で成果物・ユーザー・リポジトリ・設定を扱える。

## デフォルトリポジトリ

| パス                              | 用途      |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

Maven の `<repositories>` / `<distributionManagement>` に書く。例: [Maven クライアント](./maven-client.md)。

## 環境変数

| 変数                           | デフォルト          | 用途                         |
|--------------------------------|---------------------|------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | サーバー、UI、ストレージ、updater |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | リポジトリ、ミラー、リポジトリ単位 S3 |
| `RENOP_TOKENS`                 | `tokens.yaml`       | アカウントとトークン         |
| `RENOP_INDEX`                  | `index.json`        | 成果物インデックス           |
| `RENOP_SESSIONS`               | `sessions.json`     | ログインセッション           |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 生成                | 最初の admin パスワード      |

多くは管理 UI からも変更可。listen や TLS を変えたら再起動。
