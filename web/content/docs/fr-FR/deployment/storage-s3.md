---
title: Architecture de stockage
order: 3
category: Déploiement
description: Stockage local et objets compatibles S3 par dépôt
---

# Architecture de stockage

RenoP prend en charge le disque local et les services objet compatibles S3. Chaque dépôt choisit son backend ; les
changements sont sérialisés avec les opérations actives par le verrou du dépôt.

## Système de fichiers local

La racine est `storage_path` dans `config.yaml`, avec `storage` par défaut.

### Organisation

- **Maven/files** : `{storage_path}/{repo}/{path}`
- **Cargo** : index et archives sont isolés sous le répertoire du dépôt
- **Docker** : blobs, manifestes et références sont isolés et validés par image

Les noms physiques restent des détails internes. Utilisez les API de protocole et non un accès direct au répertoire.

### Fiabilité des écritures

- Les uploads utilisent des fichiers temporaires bornés et vérifient taille, hash et politique avant validation.
- La publication finale est atomique lorsque le système de fichiers le permet.
- Les commits de miroir, suppressions, migrations et publications GPG sont synchronisés avec les changements de backend.

---

## Stockage compatible S3

S3 convient au stockage objet géré. Un déploiement multi-nœud exige aussi une base externe et une coordination conforme
aux garanties de RenoP ; S3 seul ne transforme pas un processus unique en cluster.

### Fournisseurs

- **AWS S3**
- **MinIO**
- **Cloudflare R2**
- tout service implémentant l’API S3 requise

### Exemple (`repositories.yaml`)

```yaml
repositories:
  releases:
    name: releases
    s3:
      enabled: true
      endpoint: "https://minio.internal:9000"
      bucket: "renop-storage"
      key_prefix: "releases/"
      region: "us-east-1"
      access_key_id: "ACCESS_KEY"
      secret_access_key: "SECRET_KEY"
      force_path_style: true
      redirect_downloads: false
```

Le bucket doit exister et les identifiants doivent permettre lecture, écriture, liste et suppression sous `key_prefix`.
Utilisez TLS et un gestionnaire de secrets ; ne publiez jamais les clés dans le dépôt Git.

### Modes de téléchargement

- **Streaming proxy (`redirect_downloads: false`)** : RenoP autorise puis diffuse S3 au client. Le bucket peut rester
   privé et l’adresse S3 n’est pas exposée.
- **Redirection (`redirect_downloads: true`)** : RenoP autorise puis renvoie `302 Found` vers une URL présignée de courte
   durée, ce qui réduit la bande passante du serveur.
