# RenoP website

Static official site for RenoP (no backend). Built with the same stack as the product UI: **pnpm + Rolldown +
lightningcss**.

## Features

- Home, pricing (all free, on purpose), docs, download
- i18n + light/dark theme (styles slimmed from `frontend/renop-html`)
- Markdown docs with **auto categories** (folder-based) and **auto TOC** (headings)
- Downloads:
    - **Stable** — GitHub release assets (direct `browser_download_url`), history with 10 per page
    - **Preview** — `nightly.link` multi-arch zip, client-side extract of the selected OS/arch package

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

## Preview / nightly downloads

CI workflow [`.github/workflows/build.yml`](../.github/workflows/build.yml) uploads artifact **`dist-artifacts`**
(contents of `dist/`).  
nightly.link URL used by the site:

```text
https://nightly.link/404Setup/SRC-RenoP/workflows/build/main/dist-artifacts.zip
```
