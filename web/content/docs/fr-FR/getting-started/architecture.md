---
title: Architecture système
order: 4
category: Pour commencer
description: Architecture modulaire, flux d'E/S et cohérence des données
---

# Architecture système

- **Transfert de données en streaming** : Les flux d'artefacts circulent sans mise en mémoire tampon complète des
  fichiers.
- **Couche de données unifiée** : Support de SQLite, MySQL et PostgreSQL (avec `pgx/v5`).
- **Écritures atomiques** : Écriture dans des fichiers temporaires `.tmp` puis renommage atomique.
- **File de quarantaine GPG** : Les paquets en attente de validation sont isolés dans `.renop.tmp.gpg`.
