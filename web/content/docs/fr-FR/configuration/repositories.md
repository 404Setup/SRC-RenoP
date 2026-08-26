---
title: Dépôts et miroirs
order: 2
category: Configuration
description: Moteurs, visibilité, miroirs, migration et stockage S3
---

# Dépôts et miroirs

Les définitions résident dans `repositories.yaml`, remplaçable par `RENOP_REPOSITORIES`. L’administration modifie les
mêmes structures validées. Le nom est un slug minuscule immuable et le premier segment de l’URL.

## Exemple de configuration

```yaml
repositories:
  releases:
    name: releases
    format: maven
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: true
    mirrors: []
  crates:
    name: crates
    format: cargo
    visibility: PUBLIC
    mirrors: []
  containers:
    name: containers
    format: docker
    visibility: PRIVATE
    allow_redeployment: false
    mirrors: []
```

## Champs du dépôt

| Champ | Défaut | Description |
|:------|:-------|:------------|
| `name` | Requis | Slug immuable et préfixe URL |
| `format` | `maven` | `maven`, `maven-classic`, `files`, `cargo` ou `docker` |
| `visibility` | `PUBLIC` | `PUBLIC`, `HIDDEN` ou `PRIVATE` |
| `allow_redeployment` | `false` | Redéploiement Maven ou remplacement files/Docker si pris en charge |
| `require_gpg_signature` | `false` | Validation OpenPGP détachée obligatoire pour Maven |
| `mirrors` | `[]` | Miroirs ordonnés |
| `s3` | absent | Stockage S3 propre au dépôt |

`maven-classic` ne change que l’interface et conserve les règles Maven. `files` est non structuré, sans sommes, POM ni
signature. Maven peut migrer vers `files` puis revenir sans déplacer les objets ; le retour reconstruit le catalogue et
restaure la politique Maven.

### Visibilité

- **PUBLIC** : lecture et découverte anonymes.
- **HIDDEN** : absent des catalogues et profils pour tous ; un chemin exact reste lisible et l’administration le voit.
- **PRIVATE** : lecture, listes et écriture exigent un droit explicite. Une image Docker privée ajoute son équipe L0-L4.

## Miroirs amont

Un objet absent peut être diffusé depuis un miroir activé, puis persisté sans mettre tout le corps en mémoire. Cargo et
Docker interdisent une création locale si un nom applicable existe en amont.

```yaml
mirrors:
  - name: "central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

| Champ | Défaut | Description |
|:------|:-------|:------------|
| `name` | Requis | Nom unique dans le dépôt |
| `url` | Requis | URL de base amont |
| `persist` | `true` | Stocker les réponses réussies |
| `cache_ttl_secs` | `86400` | Durée du cache positif |
| `negative_cache` | `true` | Mettre en cache les absences prises en charge |
| `timeout_secs` | `30` | Délai d’une requête amont |
| `proxy` | `""` | Route globale, `direct` ou proxy nommé |
| `allow_artifacts` | `[]` | Règles d’autorisation selon le format |
| `deny_artifacts` | `[]` | Règles de refus prioritaires |

Les identifiants utilisent les champs structurés d’autorisation et ne doivent jamais être inclus dans `url`.

## Stockage compatible S3

Chaque dépôt choisit Disk ou son propre S3. Le verrou du dépôt sérialise un changement avec uploads, suppressions,
commits GPG et écritures de miroir.

```yaml
s3:
  enabled: true
  endpoint: "https://s3.us-east-1.amazonaws.com"
  bucket: "my-renop-bucket"
  key_prefix: "releases/"
  region: "us-east-1"
  access_key_id: "YOUR_ACCESS_KEY"
  secret_access_key: "YOUR_SECRET_KEY"
  force_path_style: false
  redirect_downloads: false
```

MinIO demande souvent `force_path_style`. Avec `redirect_downloads`, RenoP autorise puis renvoie une redirection signée
de courte durée ; sinon il diffuse l’objet lui-même.
