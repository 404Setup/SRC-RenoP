---
title: Updater
order: 7
category: API
---

# Updater

Préfixe : `/api/updater`

`GET /status` est public ; `check` / `install` / `upload` / `restart` exigent **manager**.

Même état sur `GET /api/status/instance` comme `update_state`.

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

Réponse : `application/x-protobuf`, `UpdateState` (`proto/api/v1/api.proto`).

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

| Query     | Défaut                     | Signification          |
|-----------|----------------------------|------------------------|
| `channel` | réglage `updater.channel`  | `release` ou `nightly` |

Absent / invalide → `updater.channel` (défaut `release`).

| Canal       | `info.json`                                           |
|-------------|-------------------------------------------------------|
| `nightly`   | `https://mvnc.pkg.one/update/renop/nightly/info.json` |
| `release`   | `https://mvnc.pkg.one/update/renop/stable/info.json`  |

Paquets : `…/{nightly\|stable}/{version}/{file}`.

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

Échec → 500, `{ "error": "…" }`.

## `POST /api/updater/install`

Téléchargement / extraction asynchrones via le `download_url` courant.

| Statut | Raison                                                          |
|--------|-----------------------------------------------------------------|
| 507    | Disque insuffisant                                              |
| 409    | Installation déjà en cours (`Installation already in progress`) |

```json
{"status": "started"}
```

Sondage `/status`. Fin : `ready_to_restart`.

## `POST /api/updater/upload`

Mise à jour hors ligne : zip multipart (`file` ou `package`). Doit être `.zip`.

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

### Upload multi-parties (optionnel)

Grands zip : upload découpé (manager). Sous **8 MiB** → `POST /api/updater/upload` simple.

Init/complete : **`application/x-protobuf`** (`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`). Parties : octets bruts.

Taille de partie : [storage.md](./storage.md). Utiliser `chunk_size` / `chunk_count` de l’init.

1. `POST /api/upload/chunked/` — `purpose=updater`, `filename` (`.zip`), `size`
2. `PUT /api/upload/chunked/:id/:index` (parallèle, retry-safe)
3. `POST /api/upload/chunked/:id/complete` → `ready_to_restart`

## `POST /api/updater/restart`

Si un binaire de mise à jour est en attente, l’applique puis redémarre le processus. Sinon redémarre le processus courant sans appliquer de mise à jour.

```json
{"status": "restarting"}
```
