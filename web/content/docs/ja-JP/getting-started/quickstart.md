---
title: クイックスタート
order: 3
category: はじめに
description: 初回起動手順、管理者パスワードの設定およびデフォルトリポジトリ
---

# クイックスタート

## 1. 初回起動と管理者パスワード

初回起動時、RenoP は自動的に管理者アカウント `admin` を作成します。起動前に環境変数でパスワードを指定することを推奨します：

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

環境変数を設定しなかった場合、起動時にランダムなパスワードがコンソールに表示されます。

ブラウザで `http://localhost:3000` にアクセスして管理画面にログインします。

## 2. デフォルトリポジトリ

| エンドポイント                    | 公開設定  | 用途                                           |
|:----------------------------------|:----------|:-----------------------------------------------|
| `http://localhost:3000/releases`  | `PUBLIC`  | Maven リリース版リポジトリ（上書き禁止）       |
| `http://localhost:3000/snapshots` | `PUBLIC`  | Maven スナップショットリポジトリ（上書き許可） |
| `http://localhost:3000/private`   | `PRIVATE` | Maven プライベートリポジトリ（認証必須）       |

- Cargo インデックス: `http://localhost:3000/index/`
- Docker レジストリ: `http://localhost:3000/v2/`

## 3. ヘルスチェック

```bash
curl -s http://localhost:3000/api/status/health
# 出力: "UP"
```

## 4. 主な環境変数

| 変数名                         | デフォルト値        | 説明                                   |
|:-------------------------------|:--------------------|:---------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | メイン設定ファイルパス                 |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | リポジトリおよびミラー設定ファイルパス |
| `RENOP_TOKENS`                 | `tokens.yaml`       | 初期ユーザーとトークンファイル         |
| `RENOP_INDEX`                  | `index.json`        | 検索インデックスキャッシュ             |
| `RENOP_SESSIONS`               | `sessions.bin`      | セッションデータファイル               |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | *(自動生成)*        | 初期管理者パスワード                   |
