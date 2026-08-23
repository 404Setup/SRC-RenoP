---
title: Installation & Build
order: 2
category: Getting Started
description: Downloading binaries, microarchitecture selection, and building from source
---

# Installation & Build

## 1. Download Prebuilt Binaries

You can download prebuilt archive packages directly from the web [Download Center](/download) or from official
distribution channels:

- **Stable Channel**: Recommended for production environments.
  `https://mvnc.pkg.one/update/renop/stable/`
- **Nightly Channel**: Daily builds containing the latest features and fixes.
  `https://mvnc.pkg.one/update/renop/nightly/`

## 2. x86-64 Microarchitecture Tiers

RenoP offers tiered x86-64 builds targeting specific CPU instruction sets:

| Tier                          | Instruction Set Support             | Recommended Scenarios                                                                    |
|:------------------------------|:------------------------------------|:-----------------------------------------------------------------------------------------|
| **x86-64-v1**                 | Baseline 64-bit x86                 | Compatible with all 64-bit x86 CPUs; ideal for legacy servers or generic VMs             |
| **x86-64-v2**                 | SSE3, SSSE3, SSE4.1, SSE4.2, POPCNT | Mainstream Intel and AMD CPUs released since 2008                                        |
| **x86-64-v3** *(Recommended)* | AVX, AVX2, BMI1, BMI2, FMA3         | Intel Haswell (2013+), AMD Zen 2 (2019+), and newer CPUs; **recommended for production** |
| **x86-64-v4**                 | AVX-512 foundation and extensions   | High-performance servers with AVX-512 support (Intel Skylake-X/Ice Lake, AMD Zen 4)      |
| **ARM64**                     | NEON, Crypto                        | Apple Silicon (M series), AWS Graviton, and 64-bit ARM Linux servers                     |

## 3. Verification & Execution

Every release includes a `SHA256SUMS` file. Verify the archive integrity before extraction:

```bash
# Linux
sha256sum -c SHA256SUMS --ignore-missing

# Windows (PowerShell)
Get-FileHash -Algorithm SHA256 .\renop-windows-amd64v3.zip
```

Extract the archive and run the executable directly:

- **Linux / macOS**: `./renop`
- **Windows**: `.\renop.exe`

The service listens on `0.0.0.0:3000` by default. See [Quickstart](./quickstart.md) for initial admin password setup.

## 4. Registering as a System Service

RenoP includes cross-platform service management capabilities:

```bash
# Install and register as an auto-starting system service
./renop --install

# Stop and remove the system service
./renop --uninstall
```

Supported service managers include Windows SCM, Linux systemd/OpenRC, macOS LaunchDaemons, and BSD rc.d.
See [System Service Management](../deployment/daemon.md) for details.

## 5. Building from Source

To compile RenoP from source, ensure you have the following prerequisites:

- **Go Compiler**: Must use the custom [404Setup/go](https://github.com/404Setup/go/releases) fork.
- **Frontend Toolchain**: Node.js 18+ and pnpm.
- **Scripting Environment**: PowerShell 7 (`pwsh`).
- **Protobuf**: `protoc` and `protoc-gen-go`.

### Build Commands

```powershell
# 1. Point GOROOT to 404Setup/go
$env:GOROOT = "D:\tools\go"
$env:PATH = "$env:GOROOT\bin;$env:PATH"

# 2. Install dependencies and compile frontend
pnpm install --frozen-lockfile
pnpm run build:frontend

# 3. Compile binary
pwsh ./build.ps1 c nb    # Current OS only, unzipped binary output
pwsh ./build.ps1 c       # Current OS packaged into zip release
pwsh ./build.ps1 s       # Mainstream platforms (Linux/Windows amd64/amd64v3/arm64)
pwsh ./build.ps1         # Full cross-compilation matrix
```
