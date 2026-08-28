---
title: Architecture système
order: 4
category: Pour commencer
description: Services modulaires, autorisation, stockage en flux et tâches asynchrones
---

# Architecture système

RenoP est un processus Go unique avec des frontières explicites entre transport, protocoles, autorisation, persistance
et maintenance. Le frontend intégré appelle les mêmes API bornées que les clients externes.

## Frontières des modules

```text
Browser and package clients
        |
HTTP routing, rate limits, authentication, API-token policy
        |
Maven | npm | Cargo | Docker | Files | Management services
        |
Repository gate and publication workflows
        |
Disk or S3 storage          SQL database
        |                       |
File index and mirrors      Identity, teams, audit, messages
```

- `internal/api` et les middlewares possèdent les contrats généraux, la recherche, les anomalies et les identifiants.
- Les services de format possèdent domaines/catalogues Maven, packuments npm, Sparse Index Cargo, Distribution Docker v2 et aperçus.
- La base fournit des transactions multi-dialectes pour SQLite, MySQL et PostgreSQL.
- Disk/S3 diffuse les gros corps et l’index fournit un parcours borné des métadonnées.

## Pipelines de requêtes et de tâches

### Streaming et cohérence

Les transferts diffusent entre client et Disk/S3. Hash, extraction Brotli/ZIP, cache miroir et GPG utilisent des lecteurs
bornés et fichiers temporaires. Un verrou segmenté évite qu’un changement de moteur ou stockage concurrence upload,
suppression, miroir ou publication finale.

### Authentification et autorisation

La session navigateur reste dans le cookie. Basic Auth est limité aux protocoles de paquets. Scopes et cibles exactes
d’un API Token sont croisés à chaque requête avec les droits du compte et ses niveaux L0-L4. Les ID utilisateur immuables
préservent la propriété lors d’un changement de nom.

### Travail asynchrone

Un planificateur non réentrant fusionne snapshots, nettoyages, index, compteurs et vérifications de mise à jour. Audit,
GPG, mutations de Token et surveillance de fichiers restent des files sérielles lorsque l’ordre importe. Les résultats
durables vont au centre de messages ; la progression temporaire reste dans l’UI ou les toasts.
