---
title: Démarrage rapide
order: 3
category: Pour commencer
description: Premier démarrage, mot de passe administrateur et dépôts par défaut
---

# Démarrage rapide

## 1. Premier démarrage

Définissez le mot de passe administrateur avant le lancement :

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop
```

Accédez ensuite à l'interface d'administration sur `http://localhost:3000`.

## 2. Dépôts par défaut

| URL                               | Visibilité | Rôle                                         |
|:----------------------------------|:-----------|:---------------------------------------------|
| `http://localhost:3000/releases`  | `PUBLIC`   | Dépôt Maven Release (écrasement interdit)    |
| `http://localhost:3000/snapshots` | `PUBLIC`   | Dépôt Maven Snapshot (écrasement autorisé)   |
| `http://localhost:3000/private`   | `PRIVATE`  | Dépôt Maven Privé (authentification requise) |

- Index Cargo : `http://localhost:3000/index/`
- Registre Docker : `http://localhost:3000/v2/`

## 3. Sonde de santé

```bash
curl -s http://localhost:3000/api/status/health
# Réponse : "UP"
```
