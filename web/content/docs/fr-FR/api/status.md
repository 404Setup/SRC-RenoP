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

| Champ                                                  | Signification                                        |
|--------------------------------------------------------|------------------------------------------------------|
| `version`                                              | Version du binaire                                   |
| `development`                                          | Indicateur de build de développement                 |
| `uptime`                                               | Millisecondes depuis le démarrage                    |
| `used_memory` / `total_memory`                         | Mémoire physique utilisée et totale (octets)         |
| `vss_memory`                                           | Taille de la mémoire virtuelle (octets)              |
| `renop_used_disk`                                      | Usage du stockage RenoP                              |
| `disk_used` / `disk_total`                             | Disque utilisé et total                              |
| `used_threads` / `available_threads` / `total_threads` | Goroutines et threads de concurrence                 |
| `architecture` / `os`                                  | GOARCH / GOOS                                        |
| `logical_cores` / `physical_cores`                     | Nombre de cœurs CPU logiques et physiques            |
| `failures_count`                                       | Compteur d’échecs d’exécution                        |
| `update_state`                                         | État de l’updater — voir [updater.md](./updater.md)  |
| `debug_mode`                                           | Indique si le mode de débogage était actif au départ |

## `GET /api/status/snapshots`

Échantillons historiques. Réponse : protobuf `StatusSnapshotList`.

| Champ          | Signification      |
|----------------|--------------------|
| `timestamp`    | Millisecondes Unix |
| `used_memory`  | Mémoire            |
| `used_threads` | Nombre de threads  |
| `open_files`   | Fichiers ouverts   |

Liste vide s’il n’y a pas de données (pas 404).

## API d'analyse de débogage (`/api/debug`)

Nécessite la permission **manager** et `server.debug_mode: true` dans le fichier de configuration au démarrage. Renvoie
403 si le mode de débogage est désactivé ou si les permissions sont insuffisantes.

### `GET /api/debug/memory/heap`

Exporte le profil de tas du runtime Go (format pprof).

### `GET /api/debug/memory/allocs`

Exporte le profil des allocations mémoire (format pprof).

### `GET /api/debug/memory/goroutine`

Exporte le profil de la pile de Goroutines (format pprof).

### `GET /api/debug/memory/runtime`

Renvoie la répartition de la mémoire du runtime Go (pile/hors-tas/RSS). Réponse : `application/x-protobuf`,
`RuntimeMemoryBreakdown`.
