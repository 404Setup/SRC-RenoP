---
title: Démarrage rapide
order: 3
category: Premiers pas
description: Premier lancement, mot de passe administrateur, URL des dépôts
---

# Démarrage rapide

## Premier démarrage

Au premier démarrage, RenoP crée un compte `admin`. Définissez son mot de passe via une variable d’environnement avant
de lancer le processus :

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Si la variable n’est pas définie, un mot de passe aléatoire est généré et écrit dans les journaux du serveur. Après le
démarrage, ouvrez `http://localhost:3000`.

Connectez-vous avec `admin`. Les comptes disposant des permissions manager ou admin peuvent gérer les artefacts, les
utilisateurs, les dépôts et les paramètres dans l’interface web.

## Dépôts par défaut

| Chemin                            | Rôle      |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

Configurez ces URL dans `<repositories>` ou `<distributionManagement>` Maven.
Exemples : [client Maven](./maven-client.md).

## Contrôle de santé

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## Variables d’environnement

| Variable                       | Défaut              | Rôle                                                                  |
|--------------------------------|---------------------|-----------------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Serveur, frontend, stockage, updater                                  |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Dépôts, miroirs, S3 par dépôt                                         |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Comptes et jetons                                                     |
| `RENOP_INDEX`                  | `index.json`        | Index d’artefacts                                                     |
| `RENOP_SESSIONS`               | `sessions.bin`      | Sessions de connexion (protobuf ; l’ancien `sessions.json` est migré) |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | généré              | Mot de passe du premier compte admin                                  |

La plupart des paramètres peuvent aussi être modifiés dans l’interface d’administration. Redémarrez le processus après
modification de l’adresse d’écoute ou du TLS.

## Étapes suivantes

1. [Configuration](../configuration/overview.md) — écoute, TLS, identité visuelle
2. [Dépôts et miroirs](../configuration/repositories.md)
3. [Client Maven](./maven-client.md)
4. [API HTTP](../api/README.md)
