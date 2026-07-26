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

Valeurs typiques : `frontend`, `server`, `storage`, `updater`, `index`.

`index` n’a actuellement aucun champ configurable.

### `GET /api/settings/domain/:name`

Réponse : message protobuf du domaine (Content-Type `application/x-protobuf`).

**frontend** → `FrontendConfig`

| Champ                  | Type   |
|------------------------|--------|
| `id`                   | string |
| `title`                | string |
| `description`          | string |
| `organization_website` | string |
| `organization_logo`    | string |
| `background_url`       | string |
| `icp_license`          | string |

**server** → `ServerConfig`

| Champ                 | Type            |
|-----------------------|-----------------|
| `host`                | string          |
| `port`                | uint32          |
| `ssl_enabled`         | bool            |
| `ssl_cert_path`       | string          |
| `ssl_key_path`        | string          |
| `domain`              | string          |
| `enable_compression`  | bool            |
| `file_cache_size_mb`  | uint32          |
| `max_active_requests` | uint32          |
| `trusted_proxies`     | repeated string |
| `cdn_ip_header`       | string          |

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

| Champ                | Signification                                          |
|----------------------|--------------------------------------------------------|
| `name`               | Nom du dépôt                                           |
| `visibility`         | `PUBLIC` / `HIDDEN` / `PRIVATE`                        |
| `allow_redeployment` | Autoriser l’écrasement d’artefacts existants           |
| `mirrors[]`          | Miroirs amont (url, persist, TTL, auth, allow/deny, …) |
| `s3`                 | Stockage S3-compatible optionnel                       |

### `PUT /api/settings/maven/repositories/:name`

Créer ou **remplacer entièrement**. Corps protobuf `Repository`. Le `:name` du chemin prime sur le `name` du corps.

Noms réservés : `css`, `js`, `svg`, `api`, `javadocs`, `assets`, plus caractères invalides.

Succès : 200, chaîne vide.

### `DELETE /api/settings/maven/repositories/:name`

Retirer de la config ; **ne supprime pas** les fichiers sur disque. Succès : 200, chaîne vide.
