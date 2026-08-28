---
title: Configuration de la base de données
order: 3
category: Configuration
description: Connexions SQLite, MySQL et PostgreSQL et paramètres du pool
---

# Configuration de la base de données

RenoP persiste comptes, RBAC, API Token, sessions, audit, équipes et messages dans une base de données. Configurez le
bloc `database` de `config.yaml`. Les migrations sont appliquées automatiquement au démarrage.

## SQLite (par défaut)

SQLite est intégré et ne demande aucun service externe :

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

- `dsn` accepte un chemin relatif ou absolu.
- RenoP initialise le schéma et active WAL pour les accès concurrents.

## MySQL 8.0+

Utilisez MySQL pour une base externe gérée :

```yaml
database:
  driver: "mysql"
  dsn: "renop_user:password@tcp(127.0.0.1:3306)/renop_db?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### Exigences MySQL

- MySQL 8.0 ou plus récent est recommandé.
- Utilisez `utf8mb4` avec `utf8mb4_unicode_ci` ou `utf8mb4_0900_ai_ci`.
- Le compte doit pouvoir créer et modifier les tables du schéma RenoP.

## PostgreSQL

PostgreSQL utilise le pilote `jackc/pgx/v5` :

```yaml
database:
  driver: "postgres"
  dsn: "postgres://renop_user:password@127.0.0.1:5432/renop_db?sslmode=disable"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 600
```

### Formats DSN

- **URI** : `postgres://username:password@host:port/dbname?sslmode=disable`
- **Clé-valeur** : `host=127.0.0.1 port=5432 user=renop_user password=password dbname=renop_db sslmode=disable`

En production, activez TLS selon la politique de votre fournisseur au lieu de `sslmode=disable`.

## ClickHouse 26.9+

RenoP prend en charge une instance ClickHouse autogérée avec l’API native `clickhouse.Open` de `clickhouse-go/v2`,
sans utiliser la couche de compatibilité `database/sql`. La base isolée doit exister avant le démarrage :

```yaml
database:
  driver: "clickhouse"
  dsn: "clickhouse://renop_user:password@127.0.0.1:9000/renop_db?compress=lz4"
  max_open_conns: 8
  max_idle_conns: 4
  conn_max_lifetime_sec: 600
```

Le build officiel doit inclure `EmbeddedRocksDB`. RenoP crée des tables clé-valeur mutables avec des clés composites
matérialisées sans collision et exécute les mises à jour comme mutations natives synchrones. ClickHouse 26.9 ne
proposant pas de transactions multi-instructions, RenoP sérialise les écritures et journalise des instantanés par ligne
pour restaurer toute transaction interrompue au prochain démarrage. ClickHouse Cloud n’est pas compatible avec ce mode.
La matrice est testée avec ClickHouse 26.9.1 et `clickhouse-go/v2` 2.48.0.

## Paramètres du pool

| Paramètre               | Défaut | Description                                      |
|:------------------------|:-------|:-------------------------------------------------|
| `max_open_conns`        | `25`   | Nombre maximal de connexions ouvertes            |
| `max_idle_conns`        | `25`   | Nombre maximal de connexions inactives           |
| `conn_max_lifetime_sec` | `300`  | Durée maximale avant recyclage d’une connexion   |

Dimensionnez le pool selon la limite du serveur SQL. Une valeur excessive augmente la mémoire et peut épuiser les
connexions disponibles sans améliorer le débit.
