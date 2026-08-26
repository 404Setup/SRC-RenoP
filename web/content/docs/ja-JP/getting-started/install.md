---
title: インストールとビルド
order: 2
category: はじめに
description: バイナリのダウンロード、マイクロアーキテクチャの選択、ソースコードからのビルド
---

# インストールとビルド

## 1. ビルド済みバイナリのダウンロード

Web の [ダウンロードページ](/download) または公式配布チャンネルから純 Brotli パッケージを取得できます：

- **安定版 (Stable)**: 本番環境推奨
  `https://mvnc.pkg.one/update/renop/stable/`
- **開発版 (Nightly)**: 最新機能を含む日次ビルド
  `https://mvnc.pkg.one/update/renop/nightly/`

## 2. x86-64 マイクロアーキテクチャの選択

RenoP は x86-64 CPU 向けに最適化されたビルドを提供しています：

| レベル                 | 命令セット対応                      | 推奨シナリオ                                                    |
|:-----------------------|:------------------------------------|:----------------------------------------------------------------|
| **x86-64-v1**          | 基本 64bit x86 命令                 | すべての 64bit x86 CPU に対応。古いハードウェアや仮想マシン向け |
| **x86-64-v2**          | SSE3, SSSE3, SSE4.1, SSE4.2, POPCNT | 2008年以降の主要な Intel / AMD プロセッサ                       |
| **x86-64-v3** *(推奨)* | AVX, AVX2, BMI1, BMI2, FMA3         | Intel Haswell (2013+)、AMD Zen 2 (2019+) 以降。**本番環境推奨** |
| **x86-64-v4**          | AVX-512 基本および拡張              | AVX-512 対応サーバー (Intel Skylake-X/Ice Lake, AMD Zen 4)      |
| **ARM64**              | NEON, Crypto                        | Apple Silicon、AWS Graviton、64bit ARM Linux サーバー           |

## 3. 検証と実行

各ターゲットの SHA-256 はチャンネルの `info.json` に記録されています。`.br` を展開する前に確認してください：

```bash
# Linux
sha256sum -c SHA256SUMS --ignore-missing

# Windows (PowerShell)
Get-FileHash -Algorithm SHA256 .\renop-windows-amd64v3.br
```

純 Brotli ストリームを `renop` または `renop.exe` に展開して実行します。ダウンロードページでは、
新しい `.br` をブラウザー内だけで従来の ZIP 形式へ変換することもできます。

- **Linux / macOS**: `./renop`
- **Windows**: `.\renop.exe`

デフォルトで `0.0.0.0:3000` でリッスンします。初期起動時は [クイックスタート](./quickstart.md) を参照して管理者パスワードを設定してください。

## 4. システムサービスへの登録

```bash
# システムサービスとして登録（自動起動設定）
./renop --install

# サービスを停止して削除
./renop --uninstall
```

## 5. ソースからのビルド

- **Go コンパイラ**: [404Setup/go](https://github.com/404Setup/go/releases) 専用フォークが必要です。
- **フロントエンドツール**: Node.js 18+ および pnpm。
- **シェル環境**: PowerShell 7 (`pwsh`)。

```powershell
# ビルドコマンド
pnpm install --frozen-lockfile
pnpm run build:frontend
pwsh ./build.ps1 c nb    # 現在のプラットフォーム向け単一バイナリを出力
pwsh ./build.ps1 c       # 現在のプラットフォーム向け純 Brotli パッケージ作成
```
