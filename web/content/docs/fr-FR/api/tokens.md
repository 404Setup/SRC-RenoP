---
title: Jetons
order: 3
category: API
---

# Utilisateurs et jetons d’accès

Préfixe : `/api/tokens`

Chaque point d’accès exige **manager / admin**. Les utilisateurs ordinaires changent leur mot de passe ou jeton d’upload
via
`/api/auth/profile/*`.

Un « jeton » ici est un enregistrement de compte : nom, hash de mot de passe, permissions, jeton d’upload optionnel.
Persisté dans
`tokens.yaml`.

## `GET /api/tokens`

Liste tous les comptes. Réponse : `application/x-protobuf`, `AccessTokenList`.

Forme (illustration JSON) :

```json
{
  "tokens": [
    {
      "identifier": {"type": "PERSISTENT", "value": 1},
      "name": "admin",
      "created_at": "2026-01-01T00:00:00Z",
      "description": "…",
      "expires_at": null,
      "tokens": ["<upload-token-if-any>"],
      "permissions": ["manager", "canview:*", "canupdate:*"]
    }
  ]
}
```

Les hash de mots de passe ne sont jamais renvoyés. Le tableau `tokens` contient les jetons d’upload en clair s’ils
existent. Forbidden → 403.

## `GET /api/tokens/:name`

Un compte en **protobuf** `AccessTokenDto` (`application/x-protobuf`). Noms insensibles à la casse (stockés en
minuscules). Absent → 404.

## `PUT /api/tokens/:name`

Créer ou mettre à jour. Corps : `application/x-protobuf`, `CreateAccessTokenRequest` (accepte aussi JSON).

| Champ         | Signification                                                                                            |
|---------------|----------------------------------------------------------------------------------------------------------|
| `is_create`   | `true` et le nom existe déjà → 409                                                                       |
| `secret`      | À la création, omettre pour générer un mot de passe UUID ; à la mise à jour, omettre pour ne pas changer |
| `new_name`    | Renommer ; conflit → 409                                                                                 |
| `permissions` | Remplace la liste de permissions uniquement si fourni                                                    |

Réponse : `application/x-protobuf`, `CreateAccessTokenResponse`

```protobuf
syntax = "proto3";

message CreateAccessTokenResponse {
  AccessTokenDto access_token = 1;
  string secret = 2; // présent uniquement si généré ou fourni dans cette requête
}
```

Enregistrez `secret` immédiatement après création — les mots de passe en clair ne sont pas récupérables ensuite.

## `DELETE /api/tokens/:name`

Supprime le compte. `204`. Absent → 404.

## Sessions navigateur et appareils FIDO (manager)

Les managers peuvent lister et révoquer les **sessions de connexion navigateur** et les **clés de sécurité FIDO** de
n’importe quel compte. Basic/Bearer ne sont pas des sessions. Les secrets de session ne sont jamais renvoyés.

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf. `404` si le compte n’existe pas.

### `POST /api/tokens/:name/sessions/revoke-all`

Révoque toutes les sessions navigateur de cet utilisateur. Si le manager cible **son propre** compte, la session de
cette requête est conservée. Réponse : `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Révoque une session par `public_id`. Réponse : `StatusOk` protobuf. Id manquant = no-op.

### `GET /api/auth/users/:username/fido`

Point d’accès manager pour lister les appareils FIDO d’un utilisateur. Réponse : `FidoDeviceList` protobuf.

### `DELETE /api/auth/users/:username/fido/:device_id`

Point d’accès manager pour supprimer un appareil FIDO d’un utilisateur. Réponse : `StatusOk` protobuf.

## `POST /api/tokens/:name/token`

L’admin réémet le jeton d’upload d’un utilisateur (remplace l’ancien). Réponse : `GenerateTokenResponse` protobuf
(`token: "<uuid>"`).

Même idée que `/api/auth/profile/token`, mais pour un autre utilisateur.
