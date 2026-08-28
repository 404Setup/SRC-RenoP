---
title: API de stockage et téléversement
order: 10
category: Référence API
description: Opérations directes et téléversements repris et bornés
---

# API de stockage et téléversement

Les routes directes concernent Maven et `files`. npm, Cargo et Docker utilisent leurs protocoles natifs. Chaque mutation
vérifie la capacité du Token, les droits du dépôt, son format et, pour Maven, la politique du domaine.

## Opérations directes

Le chemin canonique est `/{repo}/{path...}`. Les lectures prennent en charge validateurs HTTP et plages d’octets. Un
dépôt `HIDDEN` est absent des listes mais reste lisible par chemin exact ; un dépôt `PRIVATE` exige une autorisation.

### Télécharger

- **Requête** : `GET /{repo}/{path}` ou `HEAD /{repo}/{path}`
- Un fichier absent peut être résolu par un miroir activé et mis en cache selon la politique configurée.

### Téléverser

- **Requête** : `PUT /{repo}/{path}`
- **Authentification** : mot de passe ou API Token avec `repository:publish`, puis droits actuels du dépôt ou domaine.
- Maven n’accepte que des coordonnées et métadonnées valides sous un domaine vérifié. `files` accepte les chemins
  arbitraires assainis et leur remplacement.

### Supprimer

- **Requête** : `DELETE /{repo}/{path}`
- **Authentification** : API Token avec `repository:delete` ou autre identifiant autorisé, plus le droit de suppression.

## Téléversements découpés et repris

Les métadonnées sont en protobuf et les parties sont binaires. Le serveur contrôle la destination finale, borne taille
et sessions, puis supprime les fichiers temporaires abandonnés.

### Initialiser

- **Chemin** : `POST /api/upload/chunked/`
- **Content-Type** : `application/x-protobuf` avec `ChunkedUploadInitRequest`.
- `purpose` vaut `storage` ou `updater`. Pour le stockage, `path` commence par le nom du dépôt.

```json
{
  "purpose": "storage",
  "filename": "app-1.0.0.jar",
  "size": 524288000,
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "generate_checksums": true,
  "chunk_size": 4194304,
  "gpg_signature_expected": false
}
```

### Envoyer une partie

- **Chemin** : `PUT /api/upload/chunked/{upload_id}/{index}`
- **Content-Type** : `application/octet-stream`.
- Les parties peuvent être parallèles. Renvoyer un index accepté est idempotent ; une longueur incorrecte est refusée.

### Terminer ou annuler

- **Terminer** : `POST /api/upload/chunked/{upload_id}/complete`
- **Annuler** : `DELETE /api/upload/chunked/{upload_id}`
- Une seule finalisation gagne. Elle vérifie les parties et droits, puis valide via le verrou du dépôt.

```json
{
  "status": "created",
  "message": "",
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "release_id": ""
}
```

Avec GPG obligatoire, la réponse peut être `202 Accepted` avec `release_id` pendant la quarantaine. Pour
`purpose=updater`, le succès est `ready_to_restart` sans chemin de dépôt.
