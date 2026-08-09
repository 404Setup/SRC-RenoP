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

公式版 Go ではなく、[404Setup の Go fork](https://github.com/404Setup/go/releases)を使用してください。 PowerShell 7 と
Node.js も必要です。

1. `go.mod` の `go` バージョンを確認します。
2. OS とアーキテクチャに合う最新の `go<バージョン>` release をダウンロードします。
3. 同じ release の `SHA256SUMS` で archive を確認します。
4. 展開後、`GOROOT` を `go` ディレクトリに設定し、`GOROOT/bin` を `PATH` に追加して `go version` を 実行します。

```powershell
pwsh ./build.ps1
pwsh ./build.ps1 s
pwsh ./build.ps1 c
pwsh ./build.ps1 c nb
```

詳細はリポジトリの `README.md`。
