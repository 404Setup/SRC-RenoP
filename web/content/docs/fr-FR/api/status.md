---
title: Statut
order: 5
category: API
---

# Statut et santé

Préfixe : `/api/status`

Aucune authentification requise.

## `GET /api/status/health`

```json
"UP"
```

Sonde de vivacité (liveness).

## `GET /api/status/hash`

Hash de contenu des assets frontend en chaîne JSON (invalidation de cache).

## `GET /api/status/instance`

Réponse : `application/x-protobuf`, `InstanceStatus`.

| Champ                                                  | Signification                                       |
|--------------------------------------------------------|-----------------------------------------------------|
| `version`                                              | Version du binaire                                  |
| `development`                                          | Indicateur de build de développement                |
| `uptime`                                               | Millisecondes depuis le démarrage                   |
| `used_memory` / `total_memory`                         | Mémoire, environ MiB                                |
| `renop_used_disk`                                      | Usage du stockage RenoP                             |
| `disk_used` / `disk_total`                             | Disque                                              |
| `used_threads` / `available_threads` / `total_threads` | Threads / goroutines                                |
| `architecture` / `os`                                  | GOARCH / GOOS                                       |
| `logical_cores` / `physical_cores`                     | CPU                                                 |
| `failures_count`                                       | Compteur d’échecs d’exécution                       |
| `update_state`                                         | État de l’updater — voir [updater.md](./updater.md) |

## `GET /api/status/snapshots`

Échantillons historiques. Réponse : protobuf `StatusSnapshotList`.

| Champ          | Signification      |
|----------------|--------------------|
| `timestamp`    | Millisecondes Unix |
| `used_memory`  | Mémoire            |
| `used_threads` | Nombre de threads  |
| `open_files`   | Fichiers ouverts   |

Liste vide s’il n’y a pas de données (pas 404).
