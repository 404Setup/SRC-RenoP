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

Les pages d’image proposent un README Markdown propre au paquet. Un membre L3/L4 ou un administrateur le modifie avec
`PUT /api/docker/repositories/{repo}/images?image={name}`. La valeur JSON `description` est limitée à 512 Kio et rendue
avec la liste commune d’éléments et d’URL autorisés.

## Vérification de version

- **Chemin** : `GET /v2/` ou `HEAD /v2/`
- **Réponse** :
    - `200 OK` avec `Docker-Distribution-API-Version: registry/2.0` ;
    - `401 Unauthorized` avec `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"` si une
      authentification est requise.

---

## Jeton Bearer

- **Chemin** : `GET /v2/token` ou `GET /v2/auth`
- **Usage** : échange Basic Auth contre un jeton Docker temporaire. Un API Token exige `repository:read` pour pull,
  `repository:publish` pour push et `repository:delete` pour supprimer. La visibilité et le niveau L0-L4 sont contrôlés
  séparément avant d’accorder chaque action.

---

## Catalogue et tags

### Lister les images

- **Chemin** : `GET /v2/_catalog`
- **JSON** : `{"repositories": ["my-org/my-app"]}`

### Lister les tags

- **Chemin** : `GET /v2/:name/tags/list`
- **JSON** : `{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## Manifestes

- **Lire** : `GET /v2/:name/manifests/:reference`
- **Publier** : `PUT /v2/:name/manifests/:reference` (image créée et niveau L1 minimum)
- **Supprimer** : `DELETE /v2/:name/manifests/:reference`

Le JSON d’un manifeste est limité à 4 Mio. La même limite s’applique aux envois locaux, aux réponses miroir et aux
objets Disk/S3 persistés ; tout contenu trop volumineux est refusé avant analyse ou mise en cache.
Le digest SHA-256 déclaré doit également correspondre exactement aux octets JSON avant persistance ou diffusion.

---

## Blobs

- **Vérifier** : `HEAD /v2/:name/blobs/:digest`
- **Télécharger** : `GET /v2/:name/blobs/:digest`
- **Commencer** : `POST /v2/:name/blobs/uploads/` (`?mount=<digest>&from=<other_repo>` est pris en charge)
- **Ajouter un bloc** : `PATCH /v2/:name/blobs/uploads/:uuid`
- **Terminer** : `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`

## Verrouillage des ressources

Les administrateurs et modérateurs du dépôt utilisent `PUT /api/docker/repositories/{repo}/locks?image={name}` pour définir un verrouillage manuel et `DELETE /api/docker/repositories/{repo}/locks?image={name}` pour le retirer. Ces opérations exigent un cookie de session navigateur valide ; les jetons API ne peuvent pas gérer les verrouillages. La requête désigne le condensat immuable d’un manifeste existant ; une valeur `version` vide verrouille toute l’image.

```json
{"version":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","mode":"read","reason":"trojan"}
```

Les modes sont `write` et `read`. Les motifs publics sont `hold`, `prohibited`, `expired`, `trojan`, `abuse`, `dmca`, `reup`, `squatting` et `quality`. Un verrouillage de lecture interdit aussi les écritures : seuls les administrateurs, modérateurs du dépôt, propriétaires et collaborateurs (y compris L0) peuvent consulter les métadonnées. Personne ne peut télécharger ou monter les couches et configurations depuis cette image. Catalogues, recherches, pages de tags, profils et ressources d’équipe appliquent les mêmes règles.

Un verrouillage par condensat couvre tous les alias de tags ainsi que les manifestes enfants et blobs référencés par un index multi-architecture. Les références enregistrées survivent aux redémarrages jusqu’au retrait du verrouillage d’origine. Les réponses exposent `locks`, `inherited`, `moderator`, `member` et `version_locked`, sans identifier l’opérateur. Retirer un verrouillage manuel conserve les restrictions système et héritées. Le traitement est limité à 8,192 condensats et 64 MiB de métadonnées ; un graphe invalide ou trop volumineux renvoie `400` sans modifier le verrouillage précédent.

Les mutations bloquées renvoient `423` et `X-Renop-Error-Code: resource_locked`, notamment la réaffectation des tags, l’approbation des publications, les modifications d’équipe sous verrouillage d’image, et la suppression ou l’abandon de l’image lorsqu’une version est verrouillée. La suppression ou le remplacement d’un blob partagé vérifie également les verrouillages des autres images du dépôt. Le contenu miroir figé n’est ni actualisé ni remplacé.
