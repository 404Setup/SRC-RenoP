---
title: Authentification
order: 2
category: API
---

# Authentification et sessions

Préfixe : `/api/auth`

La configuration initiale des comptes et des jetons peut être fournie via `tokens.yaml` (`RENOP_TOKENS`). Au démarrage
du processus, les données sont automatiquement migrées et persistées dans une base de données SQLite intégrée
(`renop.db` par défaut). Les permissions sont une liste de chaînes.

## Permissions

| Valeur                | Signification                                    |
|-----------------------|--------------------------------------------------|
| `admin` / `manager`   | API d’administration (équivalentes dans le code) |
| `canview:*`           | Lecture de tous les dépôts                       |
| `canview:<repo>`      | Lecture d’un dépôt                               |
| `canupdate:*`         | Écriture sur tous les dépôts                     |
| `canupdate:<repo>`    | Écriture sur un dépôt                            |
| `allview` / `proview` | Lecture de la visibilité PRIVATE (et similaires) |
| `showing`             | Lister les racines de dépôts HIDDEN              |

Visibilité des dépôts :

- **PUBLIC** — lecture anonyme
- **HIDDEN** — fichiers lisibles ; lister la racine demande des rôles supplémentaires
- **PRIVATE** — `canview` / `allview` / `proview`, droits d’écriture sur ce dépôt, ou manager

Les écritures (PUT/POST/DELETE d’artefacts) exigent toujours `canupdate` ou manager.

## Connexion

### `POST /api/auth/login`

Corps : `application/x-protobuf`, `LoginRequest`

| Champ    | Type   | Contraintes                 |
|----------|--------|-----------------------------|
| `name`   | string | 1–128 caractères            |
| `secret` | string | 1–72 octets (limite bcrypt) |

En cas de succès : `SessionDetails` (protobuf) et cookie :

- Nom : `renop_session`
- HttpOnly, SameSite=Lax
- `Secure` en HTTPS (y compris `X-Forwarded-Proto: https` / Cloudflare visitor HTTPS)
- Max-Age ≈ 7 jours

| Statut | Raison                      |
|--------|-----------------------------|
| 401    | Mauvais nom ou mot de passe |
| 403    | Compte expiré               |
| 400    | Corps illisible             |

L’identifiant de session n’est posé que dans le cookie `renop_session`. Le champ `session_token` de la réponse de login
est vide ; les navigateurs s’appuient sur le cookie, les scripts peuvent renvoyer le même id en
`Authorization: Session …`.

## Utilisateur courant

### `GET /api/auth/me`

Renvoie `SessionDetails` (protobuf) pour la session courante. Non authentifié → 401.

| Champ           | Signification                                                                                                                                                                                                |
|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_token`  | Résumé du compte (name, created_at, permissions, …)                                                                                                                                                          |
| `permissions[]` | Rôles développés (manager reçoit une entrée `access-token:manager` en plus)                                                                                                                                  |
| `routes[]`      | Permissions de chemin issues de canview/canupdate (`route:read` / `route:write`). Les managers ont aussi `route:write` sur `*` pour que les clients reflètent les gates d’écriture sans cas spécial manager. |
| `session_token` | Défini si la requête a utilisé un en-tête `Session`                                                                                                                                                          |

L’UI d’écriture (panneau d’upload, boutons de suppression) et les PUT/POST/DELETE de stockage exigent la même permission
d’écriture effective : `admin`/`manager`, `canupdate:*` ou `canupdate:<repo>`.

Rafraîchit le cookie s’il diverge de la session courante.

## Déconnexion

### `POST /api/auth/logout`

Invalide la session et efface le cookie. `204 No Content`. Aussi 204 s’il n’y avait pas de session.

## Profil

Tous ces points d’accès exigent un utilisateur connecté.

### `PUT /api/auth/profile/password`

Corps : `application/x-protobuf`, `UpdatePasswordRequest` (accepte aussi JSON) :

| Champ          | Type   | Contrainte  |
|----------------|--------|-------------|
| `new_password` | string | 6–72 octets |

Réponse : `StatusOk` protobuf (`status: success`). Longueur invalide → 400.

### `POST /api/auth/profile/token`

Régénère le jeton d’upload (un par utilisateur ; l’ancienne valeur est remplacée). Réponse : `GenerateTokenResponse`
protobuf (`token: "<uuid>"`).

Maven / curl :

```bash
curl -u admin:UPLOAD_TOKEN -T my.jar \
  http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar
```

Le secret Basic peut être le mot de passe du compte ou le jeton d’upload, selon la configuration.

### `GET /api/auth/profile/fido`

Liste les clés de sécurité FIDO/WebAuthn enregistrées pour l’utilisateur courant.

Réponse : `application/x-protobuf`, `FidoDeviceList`

| Champ (`devices[]`) | Signification              |
|---------------------|----------------------------|
| `id`                | ID unique de l’appareil    |
| `username`          | Nom de compte              |
| `name`              | Libellé personnalisé       |
| `created_at`        | Date de création (Unix ms) |

### `POST /api/auth/profile/fido/register/begin`

Démarre une session d’enregistrement FIDO. Renvoie `session_id` et les options de création `options`.

### `POST /api/auth/profile/fido/register/finish`

Termine l’enregistrement FIDO avec `session_id`, `name` et le JSON `credential`.

### `DELETE /api/auth/profile/fido/:device_id`

Supprime l’une de vos clés FIDO par `device_id`. Réponse : `StatusOk` protobuf.

### `POST /api/auth/fido/login/begin`

Démarre une connexion sans mot de passe FIDO. `username` optionnel.

### `POST /api/auth/fido/login/finish`

Termine l’authentification FIDO, émet le cookie `renop_session` et renvoie `SessionDetails` protobuf.

### `GET /api/auth/profile/sessions`

Liste les **sessions de connexion navigateur** de l’utilisateur courant. Basic et Bearer ne créent **pas** de sessions
et n’apparaissent jamais ici. Le secret de session (valeur du cookie) n’est **jamais** renvoyé.

Réponse : `application/x-protobuf`, `SessionList`

| Champ (`sessions[]`) | Signification                                                           |
|----------------------|-------------------------------------------------------------------------|
| `public_id`          | Id opaque pour les API de révocation (pas le secret cookie)             |
| `username`           | Nom du compte                                                           |
| `ip`                 | Dernière IP client vue                                                  |
| `user_agent`         | Appareil / User-Agent à la connexion                                    |
| `created_at`         | Création (Unix ms)                                                      |
| `last_active`        | Dernière activité (Unix ms)                                             |
| `expires_at`         | Expiration d’inactivité : `last_active` + délai (typ. 7 jours, Unix ms) |
| `current`            | `true` si c’est la session de cette requête                             |

### `POST /api/auth/profile/sessions/revoke-others`

Révoque toutes les sessions navigateur de l’utilisateur **sauf** celle de cette requête. Réponse : `StatusOk` protobuf
(`status: success`).

Si l’appelant utilise Basic/Bearer (pas de session navigateur), toutes ses sessions navigateur sont révoquées.

### `DELETE /api/auth/profile/sessions/:session_id`

Supprime **une de vos** sessions par `public_id`. Réponse : `StatusOk` protobuf. Id manquant = no-op. Révoquer la
session courante efface le cookie.

## Gestion des sessions (manager)

Les managers (`admin` / `manager`) peuvent inspecter et révoquer les sessions navigateur de **n’importe quel** compte
sous `/api/tokens`.

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf pour cet utilisateur. `404` si le compte n’existe pas. `403` si l’appelant n’est pas manager.

### `POST /api/tokens/:name/sessions/revoke-all`

Révoque toutes les sessions navigateur de cet utilisateur. Si le manager cible **son propre** compte, la session de
cette requête est conservée. Réponse : `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Révoque une session de cet utilisateur par `public_id`. Réponse : `StatusOk` protobuf. Id manquant = no-op.

## Comment les clients envoient les identifiants

| Scénario                     | Approche                                                   |
|------------------------------|------------------------------------------------------------|
| UI navigateur                | Cookie (posé à la connexion)                               |
| Scripts vers les API d’admin | `Authorization: Session …` ou cookie                       |
| Maven deploy                 | Basic : `username` + mot de passe ou jeton d’upload        |
| Téléchargements CI privés    | Basic / Bearer ; les dépôts PUBLIC n’ont pas besoin d’auth |

`Bearer name:secret` se comporte comme Basic (hash de mot de passe ou jeton d’upload).  
`Bearer <upload-token>` (sans nom d’utilisateur) résout l’utilisateur via l’index de jetons.
