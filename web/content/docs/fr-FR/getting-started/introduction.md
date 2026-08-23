---
title: Introduction
order: 1
category: Pour commencer
description: Vue d'ensemble du serveur de dépôts multi-protocoles RenoP
---

# Introduction à RenoP

RenoP est un serveur de dépôts d'artefacts et de paquets multi-protocoles auto-hébergé. Développé en Go avec une
interface Web intégrée, RenoP offre une solution légère, performante et sans dépendance externe pour héberger vos
paquets en toute sécurité.

## Protocoles et écosystèmes supportés

- **Maven / Gradle** : Dépôts Release, Snapshot et Private conformes à l'arborescence Maven standard, avec extraction et
  prévisualisation Javadoc et vérification de signatures GPG.
- **Cargo (Rust)** : Protocole Sparse Index (index clairsemé), publication, téléchargement, recherche et retrait (yank)
  de crates, miroir de crates.io et prévisualisation de documentation Cargodoc.
- **Docker / OCI Registry** : Conforme aux spécifications OCI Distribution Spec v2 et Docker Registry v2, avec support
  des manifestes multi-architectures et téléversement par blocs.

## Stockage et bases de données

- **Stockage** : Système de fichiers local ou stockage objet compatible S3 (AWS S3, MinIO, Cloudflare R2, etc.).
- **Base de données** : SQLite intégrée par défaut, avec support natif pour MySQL 8.0+ et PostgreSQL.

## Fonctionnalités clés

| Fonctionnalité            | Description                                                                                          |
|:--------------------------|:-----------------------------------------------------------------------------------------------------|
| **Binaire unique**        | Prêt à l'emploi sans dépendance externe ; interface d'administration Web intégrée                    |
| **Miroirs amont**         | Proxy transparent pour Maven, Cargo et Docker avec mise en cache locale                              |
| **Contrôle d'accès RBAC** | Rôles utilisateurs, permissions granulaires par dépôt et jetons d'accès personnels (PAT)             |
| **Service système natif** | Commandes `--install` et `--uninstall` pour Windows Services, systemd, OpenRC, LaunchDaemons et rc.d |
| **Sécurité**              | Vérification des signatures OpenPGP détachées, limitation de débit et blocage des IP suspectes       |
