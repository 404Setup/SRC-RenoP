---
title: Introduction
order: 1
category: Pour commencer
description: RenoP comme plateforme intégrée et auto-hébergée de publication de paquets
---

# Introduction à RenoP

RenoP est un serveur intégré et auto-hébergé de publication et distribution. Son modèle se rapproche d’un service privé
de type Central plutôt que d’un simple dépôt Maven : un processus Go intègre interface, identités, équipes, vérifications,
catalogues, miroirs, stockage, audit et mises à jour.

## Protocoles pris en charge

- **Maven / Gradle** : domaines globaux vérifiés, catalogue moderne, présentation classique compatible, chemins Maven 2,
  miroirs, Javadoc et vérification OpenPGP détachée.
- **Cargo** : Sparse Index, propriété explicite, publication, recherche, yank/unyank, miroirs et Cargodoc.
- **Docker / OCI** : Distribution v2, réservation d’images, équipes privées, blobs découpés, mounts inter-dépôts,
  manifestes multi-architecture et miroirs.
- **Files** : stockage non structuré remplaçable avec miroirs, sans métadonnées Maven ni workflow de signature.

## Stockage et bases

- **Stockage** : streaming sur Disk local ou S3 compatible propre à chaque dépôt.
- **Base** : SQLite intégré par défaut, MySQL et PostgreSQL externes.
- **Cohérence** : les verrous de dépôt coordonnent uploads, suppressions, miroirs, GPG et changements de moteur ou stockage
  sans charger les gros objets en mémoire.

## Capacités principales

| Capacité | Description |
|:---------|:------------|
| **Service unique** | Frontend et protocoles intégrés, sans runtime applicatif séparé |
| **Identité globale** | Profils publics par nom, ID interne immuable |
| **Accès fin** | Droits par dépôt, équipes L0-L4, API Token ciblés et expirables |
| **Publication vérifiée** | Propriété de domaine Maven, conflits amont et quarantaine OpenPGP facultative |
| **Exploitation** | Service natif, tâches planifiées, audit/messages durables et mise à jour intégrée |
| **Défense** | Streaming borné, limites, bannissement, proxy fiable et aperçus sandboxés |

## Parcours documentaire

- [Installation](./install.md) — Paquets, plateformes et compilation
- [Démarrage rapide](./quickstart.md) — Premier lancement, administrateur et création de dépôts
- [Architecture](./architecture.md) — Modules, autorisation, stockage et tâches
- [Configuration](../configuration/overview.md) — Paramètres validés et variables
- [Maven et Gradle](../guides/maven-client.md) — Domaines vérifiés et clients JVM
- [Cargo](../guides/cargo-registry.md) — Sparse registry et cycle de vie des crates
- [Docker et OCI](../guides/docker-registry.md) — Réservation, connexion, push et pull
