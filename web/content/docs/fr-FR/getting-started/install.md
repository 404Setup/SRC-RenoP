---
title: Installation et compilation
order: 2
category: Pour commencer
description: Téléchargement des binaires, niveaux de microarchitecture et compilation
---

# Installation et compilation

## 1. Téléchargement des binaires

- **Version stable (Stable)** : `https://mvnc.pkg.one/update/renop/stable/`
- **Version nightly (Nightly)** : `https://mvnc.pkg.one/update/renop/nightly/`

## 2. Niveaux de microarchitecture x86-64

| Niveau                       | Instructions                        | Recommandation                                                    |
|:-----------------------------|:------------------------------------|:------------------------------------------------------------------|
| **x86-64-v1**                | Base 64 bits                        | Tout processeur x86 64 bits                                       |
| **x86-64-v2**                | SSE3, SSSE3, SSE4.1, SSE4.2, POPCNT | Processeurs depuis 2008                                           |
| **x86-64-v3** *(Recommandé)* | AVX, AVX2, BMI1, BMI2, FMA3         | Intel Haswell (2013+), AMD Zen 2 (2019+). **Idéal en production** |
| **x86-64-v4**                | AVX-512                             | Serveurs compatibles AVX-512                                      |
| **ARM64**                    | NEON, Crypto                        | Apple Silicon, AWS Graviton et serveurs ARM64                     |

## 3. Vérification et exécution

```bash
sha256sum -c SHA256SUMS --ignore-missing
./renop
```

## 4. Installation comme service système

```bash
./renop --install
./renop --uninstall
```
