---
title: API du registre npm
order: 7
category: Référence API
description: Métadonnées npm, publication, tarballs, dist-tags, équipes et routes d'administration
---

# API du registre npm

Chaque dépôt de format `npm` expose un registre JSON compatible npm sous `/{repo}/`. Le nom du paquet doit être
réservé via l'API d'administration ou l'interface avant sa première publication.

## Découverte et identité

- **Disponibilité** : `GET /{repo}/-/ping`
- **Compte courant** : `GET /{repo}/-/whoami`
- **Recherche** : `GET /{repo}/-/v1/search?text={query}&size={limit}&from={offset}`

Les erreurs de protocole utilisent des champs JSON stables `error` et `reason` :

```json
{
  "error": "not_found",
  "reason": "npm package was not found"
}
```

## Métadonnées et tarballs

- **Packument complet ou abrégé** : `GET /{repo}/{package}`
- **Tarball** : `GET /{repo}/{package}/-/{name}-{version}.tgz`
- **Publication ou modification des métadonnées** : `PUT /{repo}/{package}`

Un nom scoped peut être encodé en un paramètre, par exemple `%40example%2Flibrary`. Les packuments gèrent ETag et
Last-Modified. Une requête `application/vnd.npm.install-v1+json` reçoit des métadonnées abrégées et bornées. Les réponses
privées interdisent la mise en cache partagée.

Un document de publication contient au plus une version SemVer et une pièce jointe tarball en base64. Le JSON est
limité à 96 MiB, le tarball compressé à 64 MiB, le contenu décompressé à 512 MiB, les entrées à 100,000 et
`package.json` à 2 MiB. Un paquet conserve au plus 5,000 versions et 4 MiB de métadonnées de version cumulées. Le
serveur écrit le flux décodé en staging et ne publie jamais une archive partiellement validée.

## Dist-tags et cycle de vie

- **Lister les tags** : `GET /{repo}/-/package/{package}/dist-tags`
- **Définir un tag** : `PUT /{repo}/-/package/{package}/dist-tags/{tag}`
- **Supprimer un tag** : `DELETE /{repo}/-/package/{package}/dist-tags/{tag}`
- **Modifier ou retirer avec révision** : `PUT /{repo}/{package}/-rev/{revision}`
- **Supprimer le paquet avec révision** : `DELETE /{repo}/{package}/-rev/{revision}`

Les versions sont immuables. Le retrait et la suppression créent des pierres tombales ; un numéro publié ne peut pas
être réutilisé. Un conflit de révision renvoie `409 Conflict` et impose de relire le packument.

## API d'administration du navigateur

Les routes d'administration de même origine utilisent JSON et l'en-tête stable `X-Renop-Error-Code` en cas d'échec.

- `GET /api/npm/repositories/{repo}/packages`
- `POST /api/npm/repositories/{repo}/packages`
- `PUT /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/versions?package={package}&version={version}`
- `GET /api/npm/repositories/{repo}/owners?package={package}`
- `POST /api/npm/repositories/{repo}/owners?package={package}`
- `PUT /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `DELETE /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `GET /api/npm/repositories/{repo}/users/search?package={package}&q={query}`
- `POST /api/npm/repositories/{repo}/invitations/{id}/{accept|reject}`

Les catalogues utilisent `limit` entre 1 et 100 et un `offset` borné. Un paquet privé est omis sans appartenance ou
accès administrateur. Les détails d'équipe ne sont renvoyés qu'aux membres L3/L4 et aux administrateurs.

## Authentification et autorisation

Les clients npm acceptent Basic avec un mot de passe ou API Token, ou un API Token comme `_authToken`. Les scopes
Bearer sont intersectés avec les droits actuels du compte et les cibles exactes de dépôt, paquet ou équipe. La
publication exige un paquet existant et L1 ; métadonnées et retrait exigent L2, l'équipe L3, propriété et suppression L4.
