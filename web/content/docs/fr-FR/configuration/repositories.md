---
title: Dépôts et miroirs
order: 2
category: Configuration
description: repositories.yaml — visibilité, miroirs et S3
---

# Dépôts et miroirs

Fichier : `repositories.yaml` (surchargé par `RENOP_REPOSITORIES`).

Dépôts par défaut :

| Nom         | Rôle                            |
|-------------|---------------------------------|
| `releases`  | Releases (généralement PUBLIC)  |
| `snapshots` | Snapshots (généralement PUBLIC) |
| `private`   | Privé (PRIVATE)                 |

Clés par nom sous `repositories:`.

## Champs de dépôt

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC          # PUBLIC | HIDDEN | PRIVATE
    allow_redeployment: false
    mirrors: [ ]
    s3:
      enabled: false
      endpoint: ""
      bucket: ""
      key_prefix: ""
      region: auto
      access_key_id: ""
      secret_access_key: ""
      force_path_style: true
      redirect_downloads: false
```

| Champ                   | Description                                                                                             |
|-------------------------|---------------------------------------------------------------------------------------------------------|
| `name`                  | ID du dépôt (segment de chemin : `http://host:port/{name}/…`)                                           |
| `visibility`            | `PUBLIC` lecture anonyme, `HIDDEN` liste restreinte, `PRIVATE` permission de lecture requise            |
| `allow_redeployment`    | Autoriser l'écrasement d'un artefact existant (par défaut : releases/private `false`, snapshots `true`) |
| `require_gpg_signature` | Exiger une signature GPG détachée pour les artefacts protégés                                           |
| `mirrors`               | Proxies Maven amont (optionnel)                                                                         |
| `s3`                    | Backend compatible S3 optionnel pour ce dépôt                                                           |

La disposition Maven sous chaque dépôt est standard : `group/artifact/version/file`.

## Miroirs

En cas d'absence, les miroirs récupèrent depuis l'amont et peuvent mettre en cache le résultat.

| Champ             | Description                                                                                |
|-------------------|--------------------------------------------------------------------------------------------|
| `name`            | Nom d'affichage / configuration                                                            |
| `url`             | URL de base amont                                                                          |
| `persist`         | Persister les artefacts en cache dans le stockage                                          |
| `cache_ttl_secs`  | TTL du cache positif (secondes)                                                            |
| `negative_cache`  | Mettre en cache les réponses « non trouvé »                                                |
| `timeout_secs`    | Délai d'attente des requêtes amont                                                         |
| `authorization`   | Identifiants optionnels (`method`, `login`, `password`)                                    |
| `proxy`           | Vide = proxy global ; `direct` = connexion directe ; un nom sélectionne un proxy configuré |
| `enabled_date`    | Chaîne de date d'activation optionnelle                                                    |
| `allow_artifacts` | Si défini, seuls les motifs `group` ou `group:artifact` correspondants sont proxifiés      |
| `deny_artifacts`  | Si défini, les coordonnées correspondantes sont bloquées (ne pas combiner avec allow)      |

Méthodes d'autorisation couramment utilisées : `BASIC` / nom d'utilisateur-mot de passe, ou `Bearer` / jeton.

Les identifiants de proxy ne sont plus stockés dans `repositories.yaml`. Configurez les proxies nommés dans le domaine
global `proxy` et utilisez l’unique sélecteur `proxy` du miroir.

## Visibilité vs permissions

| Visibilité | Lecture anonyme                                                                                 | Notes                                                                     |
|------------|-------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------|
| PUBLIC     | Oui                                                                                             | Dépôt ouvert                                                              |
| HIDDEN     | La récupération de fichiers peut fonctionner ; liste racine nécessite des rôles supplémentaires |                                                                           |
| PRIVATE    | Non                                                                                             | Nécessite `canview` / `allview` / `proview`, droits d'écriture ou manager |

Les écritures nécessitent toujours `canupdate` (ou manager). Voir [Authentification](../api/authentication.md).

## Stockage compatible S3

Lorsque `s3.enabled` est à true, les artefacts de ce dépôt sont stockés dans le bucket donné. Champs typiques :

| Champ                                 | Description                                                  |
|---------------------------------------|--------------------------------------------------------------|
| `endpoint`                            | Point de terminaison API S3                                  |
| `bucket`                              | Nom du bucket                                                |
| `key_prefix`                          | Préfixe de clé d'objet optionnel dans le bucket              |
| `region`                              | Région (ou `auto`)                                           |
| `access_key_id` / `secret_access_key` | Identifiants                                                 |
| `force_path_style`                    | URL de style chemin (courant pour MinIO)                     |
| `redirect_downloads`                  | Rediriger les clients vers les URL d'objets lorsque supporté |

Lorsque `key_prefix` est vide, RenoP préserve la disposition d'objets héritée. Avant d'ajouter ou de modifier un préfixe
sur un dépôt contenant déjà des artefacts, déplacez ses objets existants vers le nouveau préfixe ; RenoP ne les migre
pas automatiquement.

## Voir aussi

- [Vue d'ensemble de la configuration](./overview.md)
- [API de stockage](../api/storage.md)
- [Client Maven](../getting-started/maven-client.md)
