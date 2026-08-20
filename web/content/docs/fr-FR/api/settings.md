---
title: Paramètres
order: 6
category: API
---

# Paramètres et configuration des dépôts

Préfixe : `/api/settings`

La lecture et l’écriture exigent **manager / admin**.

Tous les corps de requête/réponse sous ce préfixe qui portent des données structurées utilisent **
`application/x-protobuf`** (voir `proto/api/v1/api.proto`). Les corps de succès vides restent du texte brut (`""`). Les
erreurs de validation restent un court texte anglais.

Emplacements sur disque :

| Contenu               | Fichier             | Variable d’env.      |
|-----------------------|---------------------|----------------------|
| Paramètres de domaine | `config.yaml`       | `RENOP_CONFIG`       |
| Dépôts Maven          | `repositories.yaml` | `RENOP_REPOSITORIES` |

Les changements de listener / TLS nécessitent un redémarrage du processus pour s’appliquer pleinement.

## Index

### `POST /api/settings/index/rebuild`

Requête : protobuf `RebuildIndexRequest`

| Champ  | Type   | Valeurs          |
|--------|--------|------------------|
| `mode` | string | `full` \| `diff` |

| mode   | Comportement                                                 |
|--------|--------------------------------------------------------------|
| `full` | Reconstruction complète asynchrone ; vide les caches Javadoc |
| `diff` | Reconstruction différentielle                                |

Autre → 400 (`Invalid mode. Expected 'full' or 'diff'`). Succès : 200, corps chaîne vide.

## Domaines de configuration

### `GET /api/settings/domains`

Réponse : protobuf `SettingsDomainsResponse`

| Champ     | Type            |
|-----------|-----------------|
| `domains` | repeated string |

Valeurs typiques : `frontend`, `server`, `proxy`, `storage`, `updater`, `index`.

`index` n’a actuellement aucun champ configurable.

### `GET /api/settings/domain/:name`

Réponse : message protobuf du domaine (Content-Type `application/x-protobuf`).

**frontend** → `FrontendConfig`

| Champ                    | Type   |
|--------------------------|--------|
| `id`                     | string |
| `title`                  | string |
| `description`            | string |
| `organization_website`   | string |
| `organization_logo`      | string |
| `background_url`         | string |
| `icp_license`            | string |
| `public_security_filing` | string |
| `legal_notice_url`       | string |

**server** → `ServerConfig`

| Champ                 | Type            | Description                                  |
|-----------------------|-----------------|----------------------------------------------|
| `host`                | string          | Adresse IP d'écoute                          |
| `port`                | uint32          | Port d'écoute                                |
| `ssl_enabled`         | bool            | Activer TLS                                  |
| `ssl_cert_path`       | string          | Chemin du certificat TLS                     |
| `ssl_key_path`        | string          | Chemin de la clé privée TLS                  |
| `domains`             | repeated string | Noms d'hôte publics de l'instance            |
| `enable_compression`  | bool            | Activer la compression des réponses HTTP     |
| `file_cache_size_mb`  | uint32          | Limite du cache fichiers en mémoire (Mo)     |
| `max_active_requests` | uint32          | Limite de requêtes actives concurrentes      |
| `trusted_proxies`     | repeated string | Liste des proxies de confiance CIDR/IP       |
| `cdn_ip_header`       | string          | Nom de l'en-tête IP client                   |
| `cors_origins`        | repeated string | Liste des origines CORS autorisées           |
| `debug_mode`          | bool            | Activer les API de profilage de débogage     |
| `database`            | DatabaseConfig  | Paramètres de connexion à la base de données |
| `audit_log`           | AuditLogConfig  | Rétention des journaux d’audit               |
| `gpg`                 | GpgConfig       | Paramètres des serveurs de clés OpenPGP      |

**DatabaseConfig**:

| Champ                   | Type   | Description                                       |
|-------------------------|--------|---------------------------------------------------|
| `enabled`               | bool   | Activer la persistance en base de données         |
| `driver`                | string | Pilote de base de données (`sqlite3` ou `mysql`)  |
| `dsn`                   | string | DSN ou chemin de base de données (ex. `renop.db`) |
| `max_open_conns`        | int32  | Nombre maximal de connexions ouvertes             |
| `max_idle_conns`        | int32  | Nombre maximal de connexions inactives            |
| `conn_max_lifetime_sec` | int32  | Durée de vie maximale des connexions en secondes  |

**AuditLogConfig** et **GpgConfig** sont imbriqués dans `server` ; il n’existe plus de domaine `gpg` séparé.

| Champ                | Type            | Description                                    |
|----------------------|-----------------|------------------------------------------------|
| `audit_log.enabled`  | bool            | Activer la persistance du journal d’audit      |
| `audit_log.max_rows` | int32           | Nombre maximal de lignes conservées            |
| `gpg.key_servers`    | repeated string | Serveurs de clés OpenPGP HTTPS (1 à 8 entrées) |

**proxy** → `ProxyConfig`

Ce domaine contient les proxies sortants HTTP/HTTPS/SOCKS5 nommés. Un `selected` vide signifie une connexion directe ;
un nom sélectionne le proxy global correspondant.

**storage** → `StorageConfig`

| Champ                    | Type   |
|--------------------------|--------|
| `storage_path`           | string |
| `enable_javadoc_preview` | bool   |
| `javadoc_extract_path`   | string |
| `max_javadoc_size_mb`    | int64  |

**updater** → `UpdaterConfig`

| Champ     | Type   | Valeurs                                                      |
|-----------|--------|--------------------------------------------------------------|
| `channel` | string | `release` \| `nightly`                                       |
| `mode`    | string | `manual` \| `auto_check` \| `auto_install` \| `safe_install` |

**index** → `IndexDomainSettings` vide

### `PUT /api/settings/domain/:name`

**Remplacement complet** du domaine. Le corps est le même message protobuf que le GET pour ce domaine. Les champs Proto3
omis se décodent en zéros — les clients doivent envoyer la configuration complète du domaine (l’UI poste toujours l’état
complet du formulaire).

Succès : 200, chaîne vide.

Règles :

- `frontend.background_url` : si non vide, doit être joignable, IP publique, WebP, ≤ 5 MiB ; adresses privées refusées
- `storage.max_javadoc_size_mb` : doit être > 0
- `storage.storage_path` : en cas de changement de chemin, le serveur reconstruit immédiatement l’index de fichiers pour
  la nouvelle racine (et redémarre le FS watcher) ; caches Javadoc vidés
- `updater.channel` / `updater.mode` : uniquement les valeurs d’enum autorisées (vide invalide)
- `index` : rien d’inscriptible → 404

Échec de validation → 400 + court texte d’erreur anglais.

## Dépôts Maven

### `GET /api/settings/maven/repositories`

Réponse : protobuf `MavenRepositoriesResponse` (`map<string, Repository>`).

| Champ                   | Signification                                                             |
|-------------------------|---------------------------------------------------------------------------|
| `name`                  | Nom du dépôt                                                              |
| `visibility`            | `PUBLIC` / `HIDDEN` / `PRIVATE`                                           |
| `allow_redeployment`    | Autoriser l’écrasement d’artefacts existants                              |
| `require_gpg_signature` | Exiger une signature GPG détachée pour les artefacts protégés             |
| `mirrors[]`             | Miroirs amont (sélection `proxy`, url, persist, TTL, auth, allow/deny, …) |
| `s3`                    | Stockage S3-compatible optionnel                                          |

### `PUT /api/settings/maven/repositories/:name`

Créer ou **remplacer entièrement**. Corps protobuf `Repository`. Le `:name` du chemin prime sur le `name` du corps.

Noms réservés : `css`, `js`, `svg`, `api`, `javadocs`, `assets`, plus caractères invalides.

Succès : 200, chaîne vide.

### `DELETE /api/settings/maven/repositories/:name`

Retirer de la config ; **ne supprime pas** les fichiers sur disque. Succès : 200, chaîne vide.
