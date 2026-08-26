---
title: Démarrage rapide
order: 3
category: Pour commencer
description: Premier lancement, administrateur, santé et création de dépôts
---

# Démarrage rapide

## Démarrer le serveur

Au premier lancement, RenoP crée le super-administrateur `admin` dans la base. Définissez son mot de passe :

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

Sans variable, RenoP génère un mot de passe et l’affiche une fois sur stdout. Conservez-le puis ouvrez
`http://localhost:3000`. L’écoute par défaut est `0.0.0.0:3000`; en production, utilisez TLS ou un reverse proxy fiable.

## Dépôts par défaut et nouveaux dépôts

Le premier `repositories.yaml` contient trois dépôts Maven de compatibilité :

| Chemin | Visibilité | Politique |
|:-------|:-----------|:----------|
| `/releases` | `PUBLIC` | Maven, redéploiement interdit |
| `/snapshots` | `PUBLIC` | Maven, redéploiement autorisé |
| `/private` | `PRIVATE` | Maven, authentification requise |

Créez explicitement les dépôts Cargo, Docker ou `files` depuis l’administration. Les images Docker et noms Cargo sont
des ressources explicites et exigent un contrôle amont. Maven exige aussi un domaine vérifié depuis le menu du compte.

## Vérifier la santé

```bash
curl -s http://localhost:3000/api/status/health
# Output: "UP"
```

`/api/status/instance` fournit les métriques protobuf. La santé confirme seulement que le processus répond ; validez la
base et le stockage avec une opération authentifiée réelle avant d’ouvrir le trafic de production.

## Variables importantes

| Variable | Défaut | Usage |
|:---------|:-------|:------|
| `RENOP_CONFIG` | `config.yaml` | Chemin de la configuration principale |
| `RENOP_REPOSITORIES` | `repositories.yaml` | Chemin des dépôts |
| `RENOP_INDEX` | `index.json` | Chemin de l’instantané d’index |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | Généré une fois | Mot de passe initial si `admin` n’existe pas |

Comptes, sessions, équipes, API Token, audit et messages sont en base et n’ont pas de variable de chemin YAML.

## Étapes suivantes

- [Configuration](../configuration/overview.md) — TLS, base, proxy, aperçus et mise à jour
- [Dépôts et miroirs](../configuration/repositories.md) — Moteurs, visibilité, amonts, migration et S3
- [Maven et Gradle](../guides/maven-client.md) — Vérifier un domaine et configurer les clients JVM
- [Registre Cargo](../guides/cargo-registry.md) — Créer un dépôt et publier des crates
- [Registre Docker](../guides/docker-registry.md) — Créer les images avant push et configurer le client
