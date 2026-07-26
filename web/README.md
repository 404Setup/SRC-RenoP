# RenoP website

Static official site for RenoP (no backend). Built with the same stack as the product UI: **pnpm + Rolldown +
lightningcss**.

## Features

- Home, pricing (all free, on purpose), docs, download
- i18n + light/dark theme (styles slimmed from `frontend/renop-html`)
- Markdown docs with **auto categories** (folder-based) and **auto TOC** (headings)
- Downloads from the **official update host** (`mvnc.pkg.one`):
    - **Stable** — `update/renop/stable/info.json` + per-platform zips
    - **Preview** — `update/renop/nightly/info.json` + per-platform zips
    - Changelog text still comes from the GitHub API

## Develop

```powershell
cd web
pnpm install
pnpm run build
pnpm run preview   # http://localhost:4173
```

## Layout

| Path            | Role                                                |
|-----------------|-----------------------------------------------------|
| `js/`           | SPA: router, i18n, pages                            |
| `css/`          | Variables + slim layout/components + page styles    |
| `content/docs/` | Markdown sources (front matter optional)            |
| `build.mjs`     | Docs index + Rolldown + CSS + static copy → `dist/` |
| `dist/`         | Deploy this folder to any static host               |

SPA routes need a fallback to `index.html` (a `404.html` copy is emitted for hosts that support it).

## Docs front matter & i18n

Docs live under **`content/docs/{locale}/...`** (for example `en-US`, `zh-CN`):

```text
content/docs/en-US/getting-started/introduction.md
content/docs/zh-CN/getting-started/introduction.md
```

The active UI language maps to a docs locale (`zh-*` → `zh-CN`, others → `en-US`) with fallback to `en-US` if a page is
missing.

```md
---
title: My page
order: 10
category: Custom name
description: Optional blurb on the index
---
```

Category defaults to the folder under the locale root. Build emits a multi-locale `docs-index.json`.

## Official packages

CI workflow [`.github/workflows/build.yml`](../.github/workflows/build.yml) builds platform zips, then
[`.github/scripts/publish-update.ps1`](../.github/scripts/publish-update.ps1) publishes them to:

```text
https://mvnc.pkg.one/update/renop/nightly/info.json
https://mvnc.pkg.one/update/renop/stable/info.json
https://mvnc.pkg.one/update/renop/{channel}/{version}/renop-…-{os}-{arch}.zip
```

Requires repository secret `RENOP_PUBLISH_TOKEN` (Bearer token with write access to the `update` repo on that host).
