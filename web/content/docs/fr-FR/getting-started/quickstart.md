---
title: Démarrage rapide
order: 3
category: Premiers pas
description: Premier lancement, mot de passe admin, URL des dépôts
---

# Démarrage rapide

## Premier démarrage

Le premier lancement crée un compte `admin`. Mot de passe avant le start :

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Sans variable, un mot de passe aléatoire est écrit dans les logs. Ensuite `http://localhost:3000`.

Connexion : `admin`. Les managers gèrent artefacts, users, dépôts et réglages dans l’UI web.

## Dépôts par défaut

| Chemin                            | Rôle      |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

À mettre dans `<repositories>` / `<distributionManagement>` Maven. Exemples : [client Maven](./maven-client.md).

## Variables d’environnement

| Variable                       | Défaut              | Rôle                             |
|--------------------------------|---------------------|----------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Serveur, frontend, stockage, updater |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Dépôts, miroirs, S3 par dépôt    |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Comptes et jetons                |
| `RENOP_INDEX`                  | `index.json`        | Index d’artefacts                |
| `RENOP_SESSIONS`               | `sessions.json`     | Sessions de login                |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | généré              | Mot de passe du premier admin    |

Beaucoup de réglages aussi dans l’UI. Redémarrer après changement de listen / TLS.
