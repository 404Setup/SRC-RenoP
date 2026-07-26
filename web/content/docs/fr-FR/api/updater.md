---
title: Updater
order: 7
category: API
---

# Updater

Préfixe : `/api/updater`

`GET /status` est public ; `check` / `install` / `upload` / `restart` exigent **manager**.

Le même état figure aussi sur `GET /api/status/instance` comme `update_state`.

Flux typique :

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

Réponse : `application/x-protobuf`, `UpdateState` (voir `proto/api/v1/api.proto`).

| Champ                  | Signification                                                   |
|------------------------|-----------------------------------------------------------------|
| `status`               | `idle`, `available`, `downloading`, `ready_to_restart`, `error` |
| `latest_version`       | Chaîne de dernière version                                      |
| `download_url`         | URL de téléchargement du paquet                                 |
| `progress`             | 0–100 pendant le téléchargement                                 |
| `error_message`        | Défini quand `status` est `error`                               |
| `size`                 | Taille du paquet (octets)                                       |
| `estimated_disk_space` | Espace libre estimé nécessaire (octets)                         |
| `release_date`         | Chaîne de date de release                                       |
| `release_notes`        | Notes de version                                                |
| `commit_sha`           | Commit source                                                   |
| `is_release`           | Build du canal release                                          |

## `POST /api/updater/check`

| Query     | Défaut    | Signification          |
|-----------|-----------|------------------------|
| `channel` | `release` | `release` ou `nightly` |

```json
{
  "has_update": true,
  "current_version": "…",
  "latest_version": "…",
  "download_url": "…",
  "channel": "release",
  "size": 12345678,
  "estimated_disk_space": 40000000,
  "release_date": "…",
  "release_notes": "…",
  "commit_sha": "…",
  "is_release": true
}
```

Échec de la vérification → 500, `{ "error": "…" }`.

## `POST /api/updater/install`

Téléchargement et extraction asynchrones via le `download_url` courant. S’il est vide, repli sur l’URL nightly par
défaut.

| Statut | Raison                                                          |
|--------|-----------------------------------------------------------------|
| 507    | Disque insuffisant                                              |
| 409    | Installation déjà en cours (`Installation already in progress`) |

Réponse de succès immédiate :

```json
{"status": "started"}
```

Suivre la progression via `/status`. État terminé : `ready_to_restart`.

## `POST /api/updater/upload`

Mise à jour hors ligne : zip multipart. Champ de formulaire `file` ou `package` ; doit être `.zip`.

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

Ce chemin multipart en une seule requête reste le défaut pour les petits paquets et les clients hors UI.

### Upload hors ligne multi-parties — optionnel

Les grands zip du dialogue de mise à jour hors ligne du Dashboard peuvent utiliser un upload découpé concurrent via
l’API de session partagée (manager uniquement). Les paquets sous **8 MiB** utilisent encore
`POST /api/updater/upload`. Init/complete sont en **`application/x-protobuf`**
(`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`) ; les parties sont des octets bruts.

La taille de partie est choisie dynamiquement selon la taille totale (voir multi-part dans [storage.md](./storage.md)) ;
utilisez `chunk_size` / `chunk_count` de la réponse init.

1. `POST /api/upload/chunked/` avec `purpose=updater`, `filename` (doit se terminer par `.zip`), `size`
2. `PUT /api/upload/chunked/:id/:index` parallèles pour chaque partie (sûr pour les retries ; re-PUT des parties
   acceptées OK)
3. `POST /api/upload/chunked/:id/complete` — extrait le binaire et met `ready_to_restart`

Champs protobuf complete : `status=ready_to_restart`, `message=…`.

## `POST /api/updater/restart`

Remplace le binaire par la mise à jour préparée et redémarre.

Pas prêt → 400 (`No update ready to install`).

```json
{"status": "restarting"}
```

La connexion est ensuite coupée ; c’est attendu.
