---
title: インストール
order: 2
category: はじめに
description: RenoP バイナリの入手
---

# インストール

## ダウンロード

[ダウンロード](/download) ページ:

- **安定版** — `https://mvnc.pkg.one/update/renop/stable/`（OS/アーキ別 zip）
- **プレビュー** — `https://mvnc.pkg.one/update/renop/nightly/`

CI がチャネルごとに `info.json` を出す。変更履歴は GitHub から。

対応プラットフォームはビルドマトリクスどおり（Windows / Linux / FreeBSD / NetBSD / OpenBSD、amd64/arm64 など）。

## zip から

1. 自分の OS/arch の zip を取る
2. 作業ディレクトリに展開（設定はプロセス CWD 付近にできる）
3. Windows は `renop.exe`、それ以外は `./renop`

デフォルトで `0.0.0.0:3000` を listen。初回起動前に `RENOP_DEFAULT_ADMIN_PASSWORD` を設定 — [クイックスタート](./quickstart.md)。

## ソースからビルド

**Go**、**PowerShell 7**、**Node.js** が必要。

```powershell
pwsh ./build.ps1                 # 全マトリクス → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64、Windows amd64
pwsh ./build.ps1 c               # 現在のプラットフォーム
pwsh ./build.ps1 c nb            # 現在のプラットフォーム、zip なし
```

詳細はリポジトリの `README.md`。
