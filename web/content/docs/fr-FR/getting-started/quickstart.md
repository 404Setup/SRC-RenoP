---
title: Démarrage rapide
order: 3
category: Premiers pas
description: Premier lancement et URL des dépôts par défaut
---

# Démarrage rapide

## Premier démarrage

Au premier lancement, RenoP crée un compte `admin`. Définissez son mot de passe avant de démarrer le serveur :

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Si la variable n’est pas définie, un mot de passe aléatoire est affiché dans les logs. Ouvrez `http://localhost:3000` après le démarrage.

## Dépôts par défaut

| Chemin | Rôle |
|------|------|
| `http://localhost:3000/releases` | Artefacts de release |
| `http://localhost:3000/snapshots` | Artefacts snapshot |
| `http://localhost:3000/private` | Artefacts privés |

Utilisez l’une de ces URL dans `<repositories>` ou `<distributionManagement>` Maven. Exemples complets : [client Maven](./maven-client.md).

## Variables d’environnement

| Variable | Défaut | Rôle |
|----------|---------|---------|
| `RENOP_CONFIG` | `config.yaml` | Serveur, frontend et stockage |
| `RENOP_REPOSITORIES` | `repositories.yaml` | Dépôts, miroirs et S3 par dépôt |
| `RENOP_TOKENS` | `tokens.yaml` | Comptes et jetons d’accès |
| `RENOP_INDEX` | `index.json` | Index d’artefacts persisté |
| `RENOP_SESSIONS` | `sessions.json` | Sessions de connexion persistées |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | généré | Mot de passe du premier compte admin |

La plupart des réglages se modifient aussi depuis l’UI. Redémarrez le serveur après un changement de listener ou de TLS.
