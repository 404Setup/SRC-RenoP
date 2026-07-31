---
title: Installation
order: 2
category: Premiers pas
description: Télécharger le binaire RenoP
---

# Installation

## Téléchargement

[Page de téléchargement](/download), ou zip direct :

- Stable : `https://mvnc.pkg.one/update/renop/stable/`
- Preview : `https://mvnc.pkg.one/update/renop/nightly/`

## Depuis un zip

1. Extraire dans un répertoire de travail
2. `renop.exe` (Windows) ou `./renop` (Unix)

Écoute `0.0.0.0:3000` par défaut. Définir `RENOP_DEFAULT_ADMIN_PASSWORD` avant le premier démarrage — [démarrage rapide](./quickstart.md).

## Compiler

Go, PowerShell 7, Node.js.

```powershell
pwsh ./build.ps1
pwsh ./build.ps1 s
pwsh ./build.ps1 c
pwsh ./build.ps1 c nb
```

Voir le `README.md` du dépôt.
