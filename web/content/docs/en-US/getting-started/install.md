---
title: Install
order: 2
category: Getting started
description: Download the RenoP binary
---

# Install

## Download

[Download page](/download), or grab a zip:

- Stable: `https://mvnc.pkg.one/update/renop/stable/`
- Preview: `https://mvnc.pkg.one/update/renop/nightly/`

## From a zip

1. Extract into a working directory
2. Run `renop.exe` (Windows) or `./renop` (Unix)

Listens on `0.0.0.0:3000` by default. Set `RENOP_DEFAULT_ADMIN_PASSWORD` before first
start — [Quick start](./quickstart.md).

## Build from source

Use [our Go fork](https://github.com/404Setup/go/releases), not the official Go toolchain. PowerShell 7 and Node.js are
also required.

1. Check the `go` version in `go.mod`.
2. Download the newest `go<version>` release for your OS and architecture.
3. Check the archive against `SHA256SUMS` from the same release.
4. Extract it, set `GOROOT` to the `go` directory, add `GOROOT/bin` to `PATH`, and run `go version`.

```powershell
pwsh ./build.ps1                 # full matrix → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64, Windows amd64/arm64
pwsh ./build.ps1 c               # current platform
pwsh ./build.ps1 c nb            # current platform does not package as zip
```

See the repo `README.md`.
