---
title: API Docker / OCI Registry v2
order: 6
category: Référence API
description: Routes OCI Distribution v2 et Docker Registry v2
---

# API Docker / OCI Registry v2

RenoP implémente OCI Distribution Spec v2 et Docker Registry v2.

Une image est une ressource explicite. Créez-la avec `POST /api/docker/repositories/:repo/images` ou depuis la page du
dépôt avant de demander un jeton de push. Les routes de blobs et de manifestes ne créent jamais d’image implicitement.
Une image peut être privée ; elle est alors absente des catalogues non autorisés et exige une appartenance L0-L4 ou un
administrateur pour lire ses manifestes et blobs référencés.

La création renvoie `409 Conflict` si le nom normalisé existe localement ou sur un miroir activé applicable. Une
vérification amont indéterminée renvoie `503 Service Unavailable` et ne réserve pas le nom.

Les routes de gestion conservent un corps lisible et ajoutent `X-Renop-Error-Code`. L’interface traduit ce code au lieu
d’afficher le texte brut. Les routes OCI gardent la structure `errors` imposée par la spécification.

## 1. Vérification de version

- **Chemin** : `GET /v2/` ou `HEAD /v2/`
- **Réponse** :
    - `200 OK` avec `Docker-Distribution-API-Version: registry/2.0` ;
    - `401 Unauthorized` avec `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"` si une
      authentification est requise.

---

## 2. Jeton Bearer

- **Chemin** : `GET /v2/token` ou `GET /v2/auth`
- **Usage** : échange Basic Auth contre un jeton Docker temporaire. Un API Token exige `repository:read` pour pull,
  `repository:publish` pour push et `repository:delete` pour supprimer. La visibilité et le niveau L0-L4 sont contrôlés
  séparément avant d’accorder chaque action.

---

## 3. Catalogue et tags

### Lister les images

- **Chemin** : `GET /v2/_catalog`
- **JSON** : `{"repositories": ["my-org/my-app"]}`

### Lister les tags

- **Chemin** : `GET /v2/:name/tags/list`
- **JSON** : `{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## 4. Manifestes

- **Lire** : `GET /v2/:name/manifests/:reference`
- **Publier** : `PUT /v2/:name/manifests/:reference` (image créée et niveau L1 minimum)
- **Supprimer** : `DELETE /v2/:name/manifests/:reference`

---

## 5. Blobs

- **Vérifier** : `HEAD /v2/:name/blobs/:digest`
- **Télécharger** : `GET /v2/:name/blobs/:digest`
- **Commencer** : `POST /v2/:name/blobs/uploads/` (`?mount=<digest>&from=<other_repo>` est pris en charge)
- **Ajouter un bloc** : `PATCH /v2/:name/blobs/uploads/:uuid`
- **Terminer** : `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
