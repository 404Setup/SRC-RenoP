---
title: Installation
order: 2
category: Premiers pas
description: Télécharger le binaire RenoP
---

# Installation

## Téléchargement

Page [Téléchargement](/download) :

- **Stable** — `https://mvnc.pkg.one/update/renop/stable/` (zip par plateforme)
- **Preview** — `https://mvnc.pkg.one/update/renop/nightly/`

La CI publie `info.json` par canal. Les notes de version viennent de GitHub.

Plateformes = matrice de build (Windows, Linux, FreeBSD, NetBSD, OpenBSD ; amd64/arm64 et arch Linux en plus).

## Depuis un zip

1. Télécharger le zip OS/arch
2. Extraire dans un répertoire de travail (config à côté du CWD du processus)
3. `renop.exe` (Windows) ou `./renop` (Unix)

Écoute `0.0.0.0:3000` par défaut. Définir `RENOP_DEFAULT_ADMIN_PASSWORD` avant le premier démarrage — [démarrage rapide](./quickstart.md).

## Compiler

**Go**, **PowerShell 7**, **Node.js**.

```powershell
pwsh ./build.ps1                 # matrice complète → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64, Windows amd64
pwsh ./build.ps1 c               # plateforme courante
pwsh ./build.ps1 c nb            # plateforme courante, sans zip
```

Voir le `README.md` du dépôt pour protobuf et le frontend.
