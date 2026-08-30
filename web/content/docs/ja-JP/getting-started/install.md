---
title: インストールとビルド
order: 2
category: はじめに
description: Brotli package、CPU tier、検証、source build
---

# インストールとビルド

## ビルド済みバイナリ

[ダウンロードセンター](/download)または公式 channel から raw Brotli package を取得します。

- **Stable**: production 推奨 — `https://mvnc.pkg.one/update/renop/stable/`
- **Nightly**: 最新変更を含む daily build — `https://mvnc.pkg.one/update/renop/nightly/`

新形式は RFC 7932 の `.br` stream です。ダウンロードセンターは browser 内で legacy ZIP に変換できます。

## x86-64 tier

| Tier                   | Instruction                     | 推奨用途                                     |
|:-----------------------|:--------------------------------|:---------------------------------------------|
| **x86-64-v1**          | baseline x86-64                 | 旧 server と generic VM                      |
| **x86-64-v2**          | SSE3, SSSE3, SSE4.1/4.2, POPCNT | 2008 年以降の一般 Intel/AMD                  |
| **x86-64-v3** *(推奨)* | AVX, AVX2, BMI1/2, FMA3         | Intel Haswell、AMD Zen 2 以降                |
| **x86-64-v4**          | AVX-512 foundation              | AVX-512 を確認済みの high-performance server |
| **ARM64**              | NEON, Crypto                    | Apple Silicon、Graviton、ARM64 Linux         |

CPU が実際に対応する tier を選びます。v3/v4 binary は古い CPU へ動的に fall back しません。

## 検証と実行

channel の `info.json` は各 target の SHA-256 を含みます。展開前に `.br` を検証します。

```bash
# Linux
sha256sum -c SHA256SUMS --ignore-missing

# Windows (PowerShell)
Get-FileHash -Algorithm SHA256 .\renop-windows-amd64v3.br
```

raw stream を `renop` または `renop.exe` へ展開し、必要なら executable permission を付けて実行します。

- **Linux / macOS**: `./renop`
- **Windows**: `.\renop.exe`

既定は `0.0.0.0:3000` です。初期 password は[クイックスタート](./quickstart.md)を参照してください。

## システムサービス登録

```bash
# Install and register as an auto-starting system service
./renop --install

# Stop and remove the system service
./renop --uninstall
```

Windows SCM、systemd、OpenRC、LaunchDaemons、rc.d に対応します。
[サービス管理](../deployment/daemon.md)を参照してください。

## source build

必要な toolchain:

- **Go**: [404Setup/go](https://github.com/404Setup/go/releases) fork、Go 1.28+
- **Frontend**: Node.js 18+ と pnpm
- **Script**: PowerShell 7 (`pwsh`)
- **Protobuf**: `protoc` と `protoc-gen-go`

### ビルドコマンド

```powershell
# 1. Point GOROOT to 404Setup/go
$env:GOROOT = "D:\tools\go"
$env:PATH = "$env:GOROOT\bin;$env:PATH"

# 2. Install dependencies and compile frontend
pnpm install --frozen-lockfile
pnpm run build:frontend

# 3. Compile binary
pwsh ./build.ps1 c nb    # Current OS only, unzipped binary output
pwsh ./build.ps1 c       # Current OS packaged as a raw Brotli stream
pwsh ./build.ps1 s       # Mainstream platforms (Linux/Windows amd64/amd64v3/arm64)
pwsh ./build.ps1         # Full cross-compilation matrix
```

script は Brotli encoder CLI を自動 install します。compile は最大 4 task で、target 完了ごとに compression を
開始し、独立した最大 8 worker で並列処理します。
