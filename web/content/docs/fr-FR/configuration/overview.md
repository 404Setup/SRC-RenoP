---
title: Vue d’ensemble de la configuration
order: 1
category: Configuration
description: Fichiers de config, paramètres serveur et variables d’environnement
---

# Vue d’ensemble de la configuration

Config et état dans le répertoire de travail du processus. Chemins surchargeables via variables d’environnement.

## Fichiers

| Fichier             | Variable d’env       | Rôle                                                            |
|---------------------|----------------------|-----------------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | Bind serveur, TLS, marque frontend, chemin de stockage, updater |
| `repositories.yaml` | `RENOP_REPOSITORIES` | Dépôts, miroirs, S3 par dépôt                                   |
| `tokens.yaml`       | `RENOP_TOKENS`       | Utilisateurs, rôles, jetons d’upload                            |
| `index.json`        | `RENOP_INDEX`        | Cache d’index des artefacts                                     |
| `sessions.json`     | `RENOP_SESSIONS`     | Sessions de connexion navigateur                                |

Lié à l’exécution :

| Variable                       | Défaut | Rôle                                   |
|--------------------------------|--------|----------------------------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | généré | Mot de passe du premier compte `admin` |

## Structure de `config.yaml`

### `storage_path`

Répertoire racine du stockage local des artefacts (disposition par défaut sous ce chemin). Le chemin relatif par défaut
est en général `storage`.

### `server`

| Clé                   | Défaut            | Description                                                                 |
|-----------------------|-------------------|-----------------------------------------------------------------------------|
| `host`                | `0.0.0.0`         | Adresse d’écoute                                                            |
| `port`                | `3000`            | Port d’écoute                                                               |
| `ssl_enabled`         | `false`           | Activer TLS                                                                 |
| `ssl_cert_path`       | `""`              | Chemin du certificat si TLS est activé                                      |
| `ssl_key_path`        | `""`              | Chemin de la clé privée si TLS est activé                                   |
| `domains`             | `[localhost]`     | Noms d’hôte publics (UI / métadonnées + CORS par défaut)                    |
| `cors_origins`        | `[]`              | Liste CORS navigateur (vide = `domains` uniquement ; `*` = tout)            |
| `enable_compression`  | `false`           | Compression des réponses HTTP                                               |
| `file_cache_size_mb`  | `100`             | Taille du cache fichiers en mémoire (Mo)                                    |
| `max_active_requests` | `2000`            | Plafond de requêtes concurrentes (surcharge → 503)                          |
| `trusted_proxies`     | `[]`              | CIDR/IP de reverse proxies supplémentaires (loopback toujours de confiance) |
| `cdn_ip_header`       | `X-Forwarded-For` | En-tête d’IP client derrière un proxy de confiance (ex. `CF-Connecting-IP`) |

Redémarrez le processus après modification de host, port ou TLS.

### `frontend`

Marque du navigateur de dépôt embarqué :

| Clé                    | Description                          |
|------------------------|--------------------------------------|
| `id`                   | Identifiant frontend / site          |
| `title`                | Titre de page                        |
| `description`          | Courte description                   |
| `organization_website` | URL org / produit                    |
| `organization_logo`    | Chemin du logo (ex. `/svg/logo.svg`) |
| `background_url`       | Image de fond optionnelle            |
| `icp_license`          | Texte ICP / conformité optionnel     |

### `updater`

| Clé       | Défaut    | Description                                         |
|-----------|-----------|-----------------------------------------------------|
| `channel` | `release` | `release` ou `nightly`                              |
| `mode`    | `manual`  | Mode d’application des mises à jour (ex. manuel UI) |

Page [Téléchargement](/download) : mêmes sources stable / nightly.

## Interface d’administration

**manager** / **admin** : la plupart des réglages sous Settings et Repositories. Certains changements fichier exigent reload/restart.

## Stockage

- **Disque local** sous `storage_path` (défaut)
- **S3-compatible** (par dépôt dans `repositories.yaml`)

Upload peut écrire des sidecars MD5 / SHA-1 / SHA-256 / SHA-512.

Visibilité, miroirs, S3 : [Dépôts et miroirs](./repositories.md).

## Voir aussi

- [Démarrage rapide](../getting-started/quickstart.md)
- [Client Maven](../getting-started/maven-client.md)
- [Index API](../api/README.md)
