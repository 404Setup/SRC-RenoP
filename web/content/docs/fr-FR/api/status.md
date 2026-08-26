---
title: API d’état et de télémétrie
order: 9
category: Référence API
description: Santé publique, métriques, instantanés et diagnostics protégés
---

# API d’état et de télémétrie

Les réponses utilisent protobuf lorsque cela est indiqué. La santé et l’état courant sont publics ; les diagnostics
mémoire exigent un administrateur et `server.debug_mode` actif au démarrage du processus.

## 1. Santé et hash de l’interface

- **Santé** : `GET /api/status/health` renvoie `"UP"` tant que le processus répond.
- **Hash** : `GET /api/status/hash` renvoie le hash des ressources intégrées utilisé pour détecter un rechargement.

## 2. État courant de l’instance

- **Chemin** : `GET /api/status/instance`
- **Format** : protobuf `InstanceStatus`.
- **Contenu** : version, durée, mémoire RSS/VSS, disque, goroutines, CPU, échecs, debug et état de mise à jour.

### Exemple décodé

```json
{
  "version": "1.0.0",
  "uptime": 86400,
  "used_memory": 33554432,
  "vss_memory": 268435456,
  "renop_used_disk": 5242880000,
  "disk_used": 107374182400,
  "disk_total": 536870912000,
  "used_threads": 24,
  "logical_cores": 16,
  "failures_count": 0,
  "debug_mode": false
}
```

## 3. Instantanés et diagnostics

- **Instantanés** : `GET /api/status/snapshots` renvoie `StatusSnapshotList` avec temps, mémoire, goroutines, fichiers
  ouverts et VSS.
- **Profil heap** : `GET /api/debug/memory/heap` (`?gc=0` évite le GC préalable).
- **Profil allocations** : `GET /api/debug/memory/allocs`.
- **Profil goroutines** : `GET /api/debug/memory/goroutine`.
- **Détail du runtime** : `GET /api/debug/memory/runtime` (`?gc=1` exécute le GC).

```json
{
  "snapshots": [
    {
      "timestamp": 1787731200000,
      "used_memory": 33554432,
      "used_threads": 24,
      "open_files": 18,
      "vss_memory": 268435456
    }
  ]
}
```

Les profils binaires s’ouvrent avec `go tool pprof` ou Speedscope. Les diagnostics renvoient `403` si le mode debug
n’était pas actif au démarrage, même pour un administrateur.
