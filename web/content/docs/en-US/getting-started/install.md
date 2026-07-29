---
title: Install
order: 2
category: Getting started
description: Download the RenoP binary
---

# Install

## Download

[Download](/download) page:

- **Stable** — `https://mvnc.pkg.one/update/renop/stable/` (zip per platform)
- **Preview** — `https://mvnc.pkg.one/update/renop/nightly/`

CI publishes `info.json` per channel. Release notes come from GitHub.

Platforms match the build matrix: Windows, Linux, FreeBSD, NetBSD, OpenBSD; amd64/arm64 and extra Linux arches.

## From a zip

1. Download the zip for your OS/arch
2. Extract into a working directory (config is created next to the process CWD)
3. Run `renop.exe` (Windows) or `./renop` (Unix)

Listens on `0.0.0.0:3000` by default. Set `RENOP_DEFAULT_ADMIN_PASSWORD` before first start — see [Quick start](./quickstart.md).

## Requirements

- Writable working directory (config, sessions, index, default local storage)
- Outbound HTTPS if you use mirrors or the online updater
- Optional reverse proxy (nginx, Caddy, …) for TLS

## Build from source

Needs **Go**, **PowerShell 7**, **Node.js**.

```powershell
pwsh ./build.ps1                 # full matrix → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64, Windows amd64
pwsh ./build.ps1 c               # current platform
pwsh ./build.ps1 c nb            # current platform, no zip
```

Generates protobuf, bundles `frontend/renop-html`, embeds assets, builds with `CGO_ENABLED=0`. Details in the repo `README.md`.
