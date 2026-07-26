---
title: Installation
order: 2
category: Premiers pas
description: Télécharger et placer le binaire RenoP
---

# Installation

## Téléchargements officiels

Utilisez la page [Téléchargement](/download) du site :

- **Stable** — source officielle `https://mvnc.pkg.one/update/renop/stable/` (zip par plateforme)
- **Preview** — source officielle `https://mvnc.pkg.one/update/renop/nightly/` (zip par plateforme)

Les métadonnées sont publiées dans `info.json` par la CI. Le changelog reste chargé depuis GitHub.

Les plateformes prises en charge suivent la matrice de build du projet (Windows, Linux, FreeBSD, NetBSD, OpenBSD ;
amd64/arm64 et architectures Linux supplémentaires).

## Depuis une archive de release

1. Téléchargez le zip de votre plateforme
2. Extrayez-le
3. Lancez `renop.exe` sous Windows ou `./renop` sous Unix

RenoP écoute par défaut sur `0.0.0.0:3000`.

## Compiler depuis les sources

Il faut **Go**, **PowerShell 7** et **Node.js** (bundle frontend Rolldown).

```powershell
pwsh ./build.ps1                 # matrice complète, paquets dans dist/
pwsh ./build.ps1 s               # Linux amd64/arm64 et Windows amd64
pwsh ./build.ps1 c               # plateforme courante uniquement
pwsh ./build.ps1 c nb            # plateforme courante, sans archive
```

Voir le `README.md` du dépôt pour protobuf et le frontend.
