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

Listens on `0.0.0.0:3000` by default. Set `RENOP_DEFAULT_ADMIN_PASSWORD` before first start — [Quick start](./quickstart.md).

## Build from source

Needs Go, PowerShell 7, Node.js.

```powershell
pwsh ./build.ps1                 # full matrix → dist/
pwsh ./build.ps1 s               # macOS amd64/arm64, Linux amd64/arm64, Windows amd64
pwsh ./build.ps1 c               # current platform
pwsh ./build.ps1 c nb            # current platform does not package as zip
```

See the repo `README.md`.
