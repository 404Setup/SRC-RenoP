---
title: Install
order: 2
category: Getting started
description: Download and place the RenoP binary
---

# Install

## Official downloads

Use the website [Download](/download) page:

- **Stable** — GitHub release assets for each OS/arch
- **Preview** — CI artifact `dist-artifacts` from workflow `build.yml`, via [nightly.link](https://nightly.link), with
  platform package extraction in the browser

Supported platforms follow the project build matrix (Windows, Linux, FreeBSD, NetBSD, OpenBSD; amd64/arm64 and
additional Linux arches).

## From a release archive

1. Download the zip for your platform
2. Extract it into a working directory (config files are created next to the binary’s CWD)
3. Run `renop.exe` on Windows or `./renop` on Unix-like systems

RenoP listens on `0.0.0.0:3000` by default. Set `RENOP_DEFAULT_ADMIN_PASSWORD` before the first start
(see [Quick start](./quickstart.md)).

## System requirements

- A writable working directory for config, sessions, index, and (by default) local storage
- Outbound HTTPS if you use upstream mirrors or the online updater
- Optional: reverse proxy (nginx, Caddy, …) terminating TLS in front of RenoP

## Build from source

Building requires **Go**, **PowerShell 7**, and **Node.js** (frontend Rolldown bundle).

```powershell
pwsh ./build.ps1                 # full target matrix, packaged in dist/
pwsh ./build.ps1 s               # Linux amd64/arm64 and Windows amd64
pwsh ./build.ps1 c               # current platform only
pwsh ./build.ps1 c nb            # current platform, no archive
```

The build generates Protocol Buffer code, bundles `frontend/renop-html`, embeds assets, and compiles with
`CGO_ENABLED=0`. See the repository `README.md` for protobuf and frontend-only rebuild steps.
