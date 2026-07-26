---
title: Index API
order: 1
category: API
---

# RenoP HTTP API

Adresse d’écoute par défaut : `0.0.0.0:3000`.

| Chemin      | Rôle                                                               |
|-------------|--------------------------------------------------------------------|
| `/api/*`    | API d’administration (login, réglages, statut, …)                  |
| `/{repo}/…` | Disposition de dépôt Maven (téléchargement / upload / suppression) |

Les corps d’erreur sont souvent du texte brut (`Unauthorized`, `Forbidden`, `Not found`). Faites d’abord confiance au
code de statut.

## Index

| Fichier                                  | Contenu                                                               |
|------------------------------------------|-----------------------------------------------------------------------|
| [authentication.md](./authentication.md) | Connexion, sessions, permissions                                      |
| [tokens.md](./tokens.md)                 | Gestion des comptes (manager)                                         |
| [maven.md](./maven.md)                   | Parcourir, versions, badge, génération de POM                         |
| [status.md](./status.md)                 | Santé et statut d’exécution                                           |
| [settings.md](./settings.md)             | Domaines de config, dépôts, reconstruction d’index                    |
| [updater.md](./updater.md)               | Mises à jour en ligne / hors ligne                                    |
| [storage.md](./storage.md)               | GET/PUT/DELETE sur les chemins de dépôt ; upload découpé optionnel    |
| [rate-limit.md](./rate-limit.md)         | Limites IP, ban après échecs d’auth, plafond de requêtes concurrentes |

Schéma machine : [openapi.yaml](/assets/openapi.yaml).  
Définitions Proto : `proto/api/v1/api.proto` (code Go généré sous `pb/`).

## JSON et Protobuf

La plupart des points d’accès utilisent encore JSON. Ceux-ci utilisent `application/x-protobuf` :

| Point d’accès                                | Direction          |
|----------------------------------------------|--------------------|
| `POST /api/auth/login`                       | request + response |
| `GET /api/auth/me`                           | response           |
| `GET /api/tokens`                            | response           |
| `GET /api/status/instance`                   | response           |
| `GET /api/status/snapshots`                  | response           |
| `GET /api/updater/status`                    | response           |
| `POST /api/settings/index/rebuild`           | request            |
| `GET /api/settings/domains`                  | response           |
| `GET /api/settings/domain/:name`             | response           |
| `PUT /api/settings/domain/:name`             | request            |
| `GET /api/settings/maven/repositories`       | response           |
| `PUT /api/settings/maven/repositories/:name` | request            |
| `GET /api/maven/details…`                    | response           |
| `GET /api/maven/repo-details/:repo`          | response           |
| `POST /api/upload/chunked/`                  | request + response |
| `POST /api/upload/chunked/:id/complete`      | response           |

Les noms de champs suivent le proto (snake_case). Générez des clients avec `protoc`, ou suivez les codecs `protobufjs`
du frontend.

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

```bash
# Après connexion, le cookie s’appelle renop_session
curl -s -b 'renop_session=<session-id>' \
  -H 'Accept: application/x-protobuf' \
  http://localhost:3000/api/auth/me \
  -o me.bin
```

## Authentification

Porteurs pris en charge :

1. Cookie : `renop_session=<id>`
2. `Authorization: Session <id>`
3. `Authorization: Basic base64(user:password_or_upload_token)`
4. `Authorization: Bearer <user>:<secret>` ou `Bearer <upload-token>`
5. GET/HEAD uniquement : `?token=<session-or-bearer>`

Les sessions expirent après environ **7 jours** d’inactivité et se renouvellent à l’activité.

| Rôle            | Capacités                                               |
|-----------------|---------------------------------------------------------|
| Anonyme         | Lecture des dépôts PUBLIC ; API d’admin surtout 401/403 |
| Utilisateur     | Accès aux dépôts via `canview:` / `canupdate:`          |
| manager / admin | Utilisateurs, réglages, updater et autres API d’admin   |

Détails : [authentication.md](./authentication.md).

## Codes de statut

| Code | Signification                                                     |
|------|-------------------------------------------------------------------|
| 200  | OK (corps éventuellement vide ou texte brut)                      |
| 201  | Upload créé                                                       |
| 204  | Succès, pas de corps                                              |
| 400  | Paramètres / corps invalides                                      |
| 401  | Non authentifié ou identifiants invalides                         |
| 403  | Interdit, expiré, ou IP bannie après 401/403 répétés              |
| 404  | Absent ; les lectures privées peuvent renvoyer 404 au lieu de 403 |
| 409  | Conflit (nom pris, mise à jour déjà en cours)                     |
| 429  | IP anonyme au-delà de la limite de débit                          |
| 503  | Surcharge (ex. plafond de requêtes concurrentes)                  |
| 507  | Espace disque insuffisant                                         |

Limites et règles d’anomalie : [rate-limit.md](./rate-limit.md).

Version d’instance : `version` sur `GET /api/status/instance`. Pas de champ de version d’API séparé.
