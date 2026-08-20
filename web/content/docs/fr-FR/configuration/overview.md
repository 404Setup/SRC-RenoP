---
title: Vue d’ensemble de la configuration
order: 1
category: Configuration
description: Fichiers de configuration, paramètres serveur et variables d’environnement
---

# Vue d’ensemble de la configuration

Les fichiers de configuration et l’état d’exécution sont stockés dans le répertoire de travail du processus. Les chemins
peuvent être redéfinis par des variables d’environnement.

## Fichiers

| Fichier             | Variable d’environnement | Rôle                                                                                 |
|---------------------|--------------------------|--------------------------------------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`           | Adresse d’écoute, TLS, marque frontend, chemin de stockage, base de données, updater |
| `repositories.yaml` | `RENOP_REPOSITORIES`     | Dépôts, miroirs, S3 par dépôt                                                        |
| `tokens.yaml`       | `RENOP_TOKENS`           | Utilisateurs, rôles, jetons d’upload (migré automatiquement en BDD au démarrage)     |
| `renop.db`          | —                        | Base de données SQLite intégrée (stocke les jetons et sessions utilisateur)          |
| `index.json`        | `RENOP_INDEX`            | Cache d’index des artefacts                                                          |
| `sessions.bin`      | `RENOP_SESSIONS`         | Sessions de connexion navigateur (migré automatiquement en BDD au démarrage)         |

Lié à l’exécution :

| Variable                       | Défaut | Rôle                                   |
|--------------------------------|--------|----------------------------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | généré | Mot de passe du premier compte `admin` |

## Structure de `config.yaml`

### Paramètres globaux de stockage et Javadoc

| Clé                      | Défaut    | Description                                                    |
|--------------------------|-----------|----------------------------------------------------------------|
| `storage_path`           | `storage` | Répertoire racine du stockage local des artefacts              |
| `enable_javadoc_preview` | `true`    | Indique si la prévisualisation en ligne Javadoc est activée    |
| `javadoc_extract_path`   | `""`      | Chemin d'extraction Javadoc (vide utilise le cache par défaut) |
| `max_javadoc_size_mb`    | `48`      | Limite de taille maximale d'extraction Javadoc (Mo)            |

### `server`

| Clé                   | Défaut              | Description                                                                     |
|-----------------------|---------------------|---------------------------------------------------------------------------------|
| `host`                | `0.0.0.0`           | Adresse d’écoute                                                                |
| `port`                | `3000`              | Port d’écoute                                                                   |
| `ssl_enabled`         | `false`             | Activer TLS                                                                     |
| `ssl_cert_path`       | `""`                | Chemin du certificat lorsque TLS est activé                                     |
| `ssl_key_path`        | `""`                | Chemin de la clé privée lorsque TLS est activé                                  |
| `domains`             | `[localhost]`       | Noms d’hôte publics de l’instance (UI / métadonnées et CORS par défaut)         |
| `cors_origins`        | `[]`                | Liste CORS navigateur (vide = `domains` uniquement ; `*` = toute origine)       |
| `enable_compression`  | `false`             | Activer la compression des réponses HTTP                                        |
| `file_cache_size_mb`  | `16`                | Taille du cache fichiers en mémoire (Mo)                                        |
| `max_active_requests` | `512`               | Nombre maximal de requêtes concurrentes (surcharge → 503)                       |
| `trusted_proxies`     | `[]`                | CIDR/IP de reverse proxies supplémentaires (loopback toujours de confiance)     |
| `cdn_ip_header`       | `X-Forwarded-For`   | En-tête d’IP client derrière un proxy de confiance (par ex. `CF-Connecting-IP`) |
| `debug_mode`          | `false`             | Activer les API de profilage de débogage sous `/api/debug` (redémarrage requis) |
| `audit_log`           | `{}`                | Paramètres du journal d’audit                                                   |
| `gpg`                 | serveurs par défaut | Serveurs de clés OpenPGP (imbriqués dans `server`)                              |

#### CORS (`server.cors_origins`)

Détermine les valeurs `Origin` du navigateur autorisées en accès cross-origin. Les cookies de session sont renvoyés avec
`Access-Control-Allow-Credentials`.

| Valeur                    | Effet                                                                                                 |
|---------------------------|-------------------------------------------------------------------------------------------------------|
| *(vide)*                  | Uniquement les origines dont l’hôte correspond à un élément de `server.domains` (tout schéma ou port) |
| `*.pkg.one`               | Domaine apex `pkg.one` et tout sous-domaine (par ex. `mvnc.pkg.one`)                                  |
| `https://app.example.com` | Origine complète exacte (schéma, hôte et port)                                                        |
| `partner.example.com`     | Cet hôte avec tout schéma ou port                                                                     |
| `*`                       | Autoriser toute origine                                                                               |

Les configurations héritées utilisant la forme singulière `domain: example.com` se chargent encore et sont migrées vers
`domains: [example.com]`.

Redémarrez le processus après modification de `host`, `port` ou des paramètres TLS.

#### GPG (`server.gpg`)

`server.gpg.key_servers` contient 1 à 8 serveurs de clés OpenPGP en HTTPS. Les paramètres GPG sont désormais dans
`server` ; l’ancien domaine `gpg` séparé n’est plus exposé.

### `proxy`

La section facultative `proxy` définit des proxies sortants HTTP/HTTPS/SOCKS5 nommés. Une valeur `selected` vide (valeur
par défaut) signifie une connexion directe. Un miroir peut hériter de ce choix, utiliser `direct` pour le contourner ou
sélectionner un proxy nommé.

### `database`

Paramètres de connexion à la base de données pour le stockage des comptes et des sessions :

| Clé                     | Défaut     | Description                                                   |
|-------------------------|------------|---------------------------------------------------------------|
| `enabled`               | `true`     | Activer la persistance en base de données intégrée ou externe |
| `driver`                | `sqlite3`  | Nom du pilote de base de données (`sqlite3` ou `mysql`)       |
| `dsn`                   | `renop.db` | DSN ou chemin de fichier de base de données (ex. `renop.db`)  |
| `max_open_conns`        | `25`       | Nombre maximal de connexions ouvertes                         |
| `max_idle_conns`        | `25`       | Nombre maximal de connexions inactives                        |
| `conn_max_lifetime_sec` | `300`      | Durée de vie maximale des connexions en secondes              |

### `frontend`

Champs de marque du navigateur de dépôt embarqué :

| Clé                      | Description                                           |
|--------------------------|-------------------------------------------------------|
| `id`                     | Identifiant frontend / site                           |
| `title`                  | Titre de page                                         |
| `description`            | Courte description                                    |
| `organization_website`   | URL de l’organisation ou du produit                   |
| `organization_logo`      | Chemin du logo (par ex. `/svg/logo.svg`)              |
| `background_url`         | URL d’image de fond optionnelle                       |
| `icp_license`            | Texte ICP ou de conformité optionnel                  |
| `public_security_filing` | Enregistrement de sécurité publique chinois optionnel |
| `legal_notice_url`       | URL facultative des mentions légales                  |

### `updater`

| Clé       | Défaut    | Description                                                                   |
|-----------|-----------|-------------------------------------------------------------------------------|
| `channel` | `release` | `release` ou `nightly`                                                        |
| `mode`    | `manual`  | Mode d’application des mises à jour (par ex. installation manuelle dans l’UI) |

La page [Téléchargement](/download) du site et les mises à jour dans l’instance utilisent la même classe de sources
stable et nightly.

## Interface d’administration

Les comptes disposant des permissions **manager** ou **admin** peuvent modifier la plupart des paramètres sous Settings
et Repositories. Certains changements de configuration nécessitent un rechargement ou un redémarrage du processus après
écriture du fichier. Consultez la documentation de chaque domaine de configuration.

## Stockage

- **Disque local** sous `storage_path` (mode par défaut)
- **Stockage objet compatible S3**, configuré par dépôt dans `repositories.yaml`

Les uploads peuvent écrire des fichiers de checksum sidecars MD5 / SHA-1 / SHA-256 / SHA-512.

Pour la visibilité, les miroirs et les champs S3, voir [Dépôts et miroirs](./repositories.md).

## Voir aussi

- [Démarrage rapide](../getting-started/quickstart.md)
- [Client Maven](../getting-started/maven-client.md)
- [Index API](../api/README.md)
