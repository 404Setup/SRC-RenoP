---
title: インストール
order: 2
category: はじめに
description: RenoP バイナリの入手と配置
---

# インストール

## 公式ダウンロード

Web サイトの [ダウンロード](/download) ページを利用します。

- **安定版** — OS / アーキテクチャごとの GitHub リリース資産
- **プレビュー** — ワークフロー `build.yml` の CI 成果物 `dist-artifacts`（[nightly.link](https://nightly.link)
  経由）。ブラウザ内で対象プラットフォームのパッケージを展開

サポート対象はプロジェクトのビルドマトリクスに従います（Windows、Linux、FreeBSD、NetBSD、OpenBSD。amd64 / arm64 および追加の
Linux アーキテクチャ）。

## リリースアーカイブから

1. 自分のプラットフォーム用 zip をダウンロード
2. 展開する
3. Windows では `renop.exe`、Unix 系では `./renop` を実行

デフォルトでは RenoP は `0.0.0.0:3000` で待ち受けます。

## ソースからビルド

ビルドには **Go**、 **PowerShell 7**、 **Node.js**（フロントエンドの Rolldown バンドル）が必要です。

```powershell
pwsh ./build.ps1                 # 全ターゲットマトリクス、dist/ にパッケージ
pwsh ./build.ps1 s               # Linux amd64/arm64 と Windows amd64
pwsh ./build.ps1 c               # 現在のプラットフォームのみ
pwsh ./build.ps1 c nb            # 現在のプラットフォーム、アーカイブなし
```

protobuf とフロントエンドの詳細はリポジトリの `README.md` を参照してください。
