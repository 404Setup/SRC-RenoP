---
title: インストール
order: 2
category: はじめに
description: RenoP バイナリの入手
---

# インストール

## ダウンロード

[ダウンロードページ](/download)、または zip を直接取得:

- 安定版: `https://mvnc.pkg.one/update/renop/stable/`
- プレビュー: `https://mvnc.pkg.one/update/renop/nightly/`

## zip から

1. 作業ディレクトリに展開
2. Windows は `renop.exe`、それ以外は `./renop`

デフォルトで `0.0.0.0:3000`。初回起動前に `RENOP_DEFAULT_ADMIN_PASSWORD` を設定 — [クイックスタート](./quickstart.md)。

## ソースからビルド

Go、PowerShell 7、Node.js が必要。

```powershell
pwsh ./build.ps1
pwsh ./build.ps1 s
pwsh ./build.ps1 c
pwsh ./build.ps1 c nb
```

詳細はリポジトリの `README.md`。
