---
title: Index de l’API
order: 1
category: Référence API
description: Vue d’ensemble de l’API HTTP, REST et RPC de RenoP
---

# API HTTP de RenoP

RenoP fournit une API HTTP complète pour l’administration, les intégrations clientes et la supervision. Par défaut,
le serveur écoute sur `http://localhost:3000`.

## Structure des routes

| Préfixe                         | Usage                                                                 |
|:--------------------------------|:----------------------------------------------------------------------|
| `/api/*`                        | Gestion : authentification, comptes, paramètres, état et messages     |
| `/{repo}/*`                     | Téléversement, téléchargement et suppression selon le format du dépôt |
| `/index/*` ou `/{repo}/index/*` | Index clairsemé Cargo                                                 |
| `/v2/*`                         | Distribution Docker et OCI v2                                        |
| `/javadoc/*`                    | Consultation sécurisée des Javadocs                                   |
| `/cargodoc/*`                   | Consultation sécurisée des Cargodocs                                  |

## Formats et Protobuf

La plupart des API de gestion utilisent JSON. Les routes à haut débit prennent aussi en charge Google Protocol
Buffers avec `application/x-protobuf`.

Utilisez `Accept: application/x-protobuf` ou `Content-Type: application/x-protobuf` selon la route. Le contrat source
se trouve dans `proto/api/v1/api.proto`.

## Transports d’authentification

1. **Cookie navigateur** : `renop_session=<session_id>`. Ce secret HttpOnly n’est accepté ni dans les en-têtes ni dans
   les URL.
2. **Jeton API Bearer** : `Authorization: Bearer <token>`. Les capacités du jeton sont toujours croisées avec les
   autorisations actuelles du compte.
3. **Basic Auth pour les protocoles de paquets** : `Authorization: Basic <base64(user:password_or_token)>`.

Basic Auth ne peut pas appeler les API de gestion. Les identifiants dans les paramètres d’URL et
`Authorization: Session` sont refusés.

## Codes HTTP usuels

| Code                      | Signification     | Usage                                                        |
|:--------------------------|:------------------|:-------------------------------------------------------------|
| `200 OK`                  | Réussite          | Requête traitée avec un corps de réponse                      |
| `201 Created`             | Créé              | Ressource ou téléversement initialisé                         |
| `204 No Content`          | Réussite          | Requête traitée sans corps                                    |
| `400 Bad Request`         | Requête invalide  | Paramètres ou corps invalides                                 |
| `401 Unauthorized`        | Non authentifié   | Identifiants absents ou invalides                             |
| `403 Forbidden`           | Interdit          | Autorisation insuffisante ou adresse IP temporairement bannie |
| `404 Not Found`           | Introuvable       | Ressource absente                                             |
| `409 Conflict`            | Conflit           | État incompatible ou ressource déjà existante                 |
| `429 Too Many Requests`   | Limite atteinte   | Débit autorisé dépassé                                        |
| `503 Service Unavailable` | Indisponible      | Service surchargé ou dépendance temporairement indisponible   |

## Index de référence

- [API d’authentification](./authentication.md)
- [Jetons API et utilisateurs](./tokens.md)
- [API Maven](./maven.md)
- [API Cargo](./cargo.md)
- [API Docker / OCI](./docker.md)
- [API du centre de messages](./messages.md)
- [API de stockage et téléversement](./storage.md)
- [API des paramètres](./settings.md)
- [API d’état et de télémétrie](./status.md)
- [API de cryptographie GPG](./gpg.md)
- [Limitation de débit](./rate-limit.md)
- [API de mise à jour](./updater.md)
