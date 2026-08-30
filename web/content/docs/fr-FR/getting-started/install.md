---
title: Installation et compilation
order: 2
category: Pour commencer
description: Paquets Brotli, niveaux CPU, vérification et compilation depuis les sources
---

# Installation et compilation

## Télécharger un binaire

Téléchargez un paquet Brotli brut depuis le [Centre de téléchargement](/download) ou les canaux officiels :

- **Stable** : recommandé en production — `https://mvnc.pkg.one/update/renop/stable/`
- **Nightly** : derniers changements quotidiens — `https://mvnc.pkg.one/update/renop/nightly/`

Les nouveaux paquets sont des flux RFC 7932 `.br`. Le centre peut les convertir côté navigateur vers l’ancien ZIP.

## Niveaux x86-64

| Niveau                       | Instructions                    | Usage recommandé                         |
|:-----------------------------|:--------------------------------|:-----------------------------------------|
| **x86-64-v1**                | Base x86-64                     | Ancien matériel et VM générique          |
| **x86-64-v2**                | SSE3, SSSE3, SSE4.1/4.2, POPCNT | Intel/AMD courants depuis 2008           |
| **x86-64-v3** *(recommandé)* | AVX, AVX2, BMI1/2, FMA3         | Intel Haswell, AMD Zen 2 et plus récents |
| **x86-64-v4**                | Fondation AVX-512               | Serveurs dont AVX-512 est confirmé       |
| **ARM64**                    | NEON, Crypto                    | Apple Silicon, Graviton et Linux ARM64   |

Choisissez le niveau réellement pris en charge. Un binaire v3/v4 ne dispose pas d’un repli dynamique sur un CPU plus
ancien.

## Vérifier et exécuter

Chaque cible de `info.json` contient son SHA-256. Vérifiez le `.br` avant décompression :

```bash
# Linux
sha256sum -c SHA256SUMS --ignore-missing

# Windows (PowerShell)
Get-FileHash -Algorithm SHA256 .\renop-windows-amd64v3.br
```

Décompressez le flux vers `renop` ou `renop.exe`, rendez-le exécutable si nécessaire, puis lancez-le :

- **Linux / macOS** : `./renop`
- **Windows** : `.\renop.exe`

L’écoute par défaut est `0.0.0.0:3000`. Consultez le [Démarrage rapide](./quickstart.md) pour le mot de passe initial.

## Installer comme service

```bash
# Install and register as an auto-starting system service
./renop --install

# Stop and remove the system service
./renop --uninstall
```

Windows SCM, systemd, OpenRC, LaunchDaemons et rc.d sont pris en charge. Voir
[Gestion du service](../deployment/daemon.md).

## Compiler depuis les sources

Prérequis :

- **Go** : fork [404Setup/go](https://github.com/404Setup/go/releases), Go 1.28 ou plus récent ;
- **Frontend** : Node.js 18+ et pnpm ;
- **Scripts** : PowerShell 7 (`pwsh`) ;
- **Protobuf** : `protoc` et `protoc-gen-go`.

### Commandes de compilation

```powershell
# 1. Point GOROOT to 404Setup/go
$env:GOROOT = "D:\tools\go"
$env:PATH = "$env:GOROOT\bin;$env:PATH"

# 2. Install dependencies and compile frontend
pnpm install --frozen-lockfile
pnpm run build:frontend

# 3. Compile binary
pwsh ./build.ps1 c nb    # Current OS only, unzipped binary output
pwsh ./build.ps1 c       # Current OS packaged as a raw Brotli stream
pwsh ./build.ps1 s       # Mainstream platforms (Linux/Windows amd64/amd64v3/arm64)
pwsh ./build.ps1         # Full cross-compilation matrix
```

Le script installe automatiquement l’outil Go de compression Brotli. Les compilations sont bornées à quatre tâches ;
les compressions démarrent dès qu’une cible est prête et utilisent jusqu’à huit workers parallèles indépendants.
