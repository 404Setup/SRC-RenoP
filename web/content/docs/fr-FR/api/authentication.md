---
title: API d’authentification
order: 2
category: Référence API
description: Sessions, profils, méthodes de connexion, récupération et révocation
---

# API d’authentification

Le navigateur utilise le cookie HttpOnly `renop_session`. Son secret n’est jamais renvoyé par les API de profil ou de
sessions et il est refusé dans les en-têtes et URL. Les paramètres de sécurité privés exigent une session navigateur,
jamais un mot de passe ou un API Token.

## Connexion par mot de passe ou e-mail

- **Chemin** : `POST /api/auth/login`
- **Authentification** : aucune.
- **Corps** : protobuf `LoginRequest`; les noms JSON figurent ci-dessous. `name` accepte le nom du compte ou son e-mail
  privé.

### Requête

```json
{
  "name": "admin",
  "secret": "your_password"
}
```

### Résultat de session

La réussite définit `renop_session` avec `HttpOnly`, `SameSite=Lax` et `Secure` sous HTTPS. Le protobuf `SessionDetails`
contient les droits et routes du compte, mais laisse `session_token` vide.

## Connexion Passkey et GitHub

- **Début Passkey** : `POST /api/auth/fido/login/begin`
- **Fin Passkey** : `POST /api/auth/fido/login/finish`
- **Début GitHub** : `GET /api/auth/github/start`
- **Callback GitHub** : `GET /api/auth/github/callback`
- **Disponibilité GitHub** : `GET /api/auth/github/status`

GitHub n’apparaît qu’après configuration OAuth. RenoP demande la lecture du compte et des organisations, conserve les
identifiants immuables et l’instantané des principals, mais jamais le jeton d’accès OAuth.

## Compte courant et profils publics

- **Session courante** : `GET /api/auth/me`
- **Profil privé** : `GET /api/auth/profile`
- **Modifier nom ou pseudonyme** : `PUT /api/auth/profile`
- **Modifier le mot de passe** : `PUT /api/auth/profile/password`
- **Déconnexion** : `POST /api/auth/logout`
- **Profil public** : `GET /api/users/:username/profile`
- **Appartenances** : `GET /api/users/:username/memberships?format=cargo|docker|maven|npm`

Les routes visibles utilisent le nom du compte ; l’identifiant immuable reste interne. Les dépôts `HIDDEN` sont omis,
et les appartenances privées ne sont visibles que par un lecteur autorisé.

## Sécurité du compte

Ces routes exigent la session navigateur courante et renvoient `Cache-Control: no-store`.

### E-mail et politique du mot de passe

- **Lire l’état** : `GET /api/auth/profile/security`
- **Définir l’e-mail** : `PUT /api/auth/profile/email`
- **Activer ou désactiver le mot de passe** : `PUT /api/auth/profile/password-login`
- Le mot de passe ne peut être désactivé que si Passkey ou GitHub reste lié. Son activation exige un mot de passe
  défini.

### Codes de récupération

- **Générer** : `POST /api/auth/profile/recovery-codes`
- **Réinitialiser** : `POST /api/auth/recovery/password`
- La génération montre une fois douze codes. Seuls des vérificateurs Argon2id sont stockés. Quatre codes distincts et
  inutilisés sont consommés atomiquement ; les sessions sont révoquées et la connexion par mot de passe réactivée.

```json
{
  "identifier": "admin@example.com",
  "codes": ["CODE-ONE", "CODE-TWO", "CODE-THREE", "CODE-FOUR"],
  "new_password": "new_secure_password"
}
```

## Gestion des méthodes de connexion

- **Lister les Passkeys** : `GET /api/auth/profile/fido`
- **Enregistrer** : `POST /api/auth/profile/fido/register/begin` puis
  `POST /api/auth/profile/fido/register/finish`
- **Supprimer** : `DELETE /api/auth/profile/fido/:device_id`
- **Lire l’identité GitHub** : `GET /api/auth/profile/github`
- **Déconnecter GitHub** : `DELETE /api/auth/profile/github`

La dernière méthode de connexion fonctionnelle ne peut être supprimée ni désactivée.

## Sessions navigateur

- **Lister** : `GET /api/auth/profile/sessions`
- **Révoquer une session** : `DELETE /api/auth/profile/sessions/:session_id`
- **Révoquer les autres** : `POST /api/auth/profile/sessions/revoke-others`

La liste expose un ID public, la méthode, les dates, l’IP et l’agent utilisateur, jamais le secret du cookie.

## Fermeture définitive du compte

`GET /api/auth/profile/retirement` renvoie les conditions de fermeture du compte courant. Cette route et
`DELETE /api/auth/profile/retirement` exigent une session de navigateur. La suppression accepte du JSON :

```json
{"confirmation":"alice"}
```

La confirmation doit correspondre exactement au nom d'utilisateur. La fermeture réussie renvoie `204 No Content`.
Une confirmation incorrecte renvoie `400` et `ACCOUNT_RETIREMENT_CONFIRMATION`. Des conditions non remplies
renvoient `409`, `ACCOUNT_RETIREMENT_BLOCKED` et le plan actualisé, avec les champs `eligible`,
`protected_role`, `super_team_owner_count`, `maven_domain_owner_count`, `package_owner_count` et
`pending_review_count`.

Les administrateurs système et les modérateurs de dépôt ne peuvent pas fermer leur compte. Aucun rôle T4 d'équipe
globale, domaine Maven actif détenu, paquet non déprécié détenu au niveau L4 ou demande de révision en attente ne doit
subsister. Transférez la propriété, fermez les domaines ou dépréciez définitivement les paquets avant de réessayer.

La fermeture réserve définitivement le compte et son nom, retire les adhésions, libère la connexion GitHub et supprime
les Passkeys, sessions, API tokens, photo, codes de récupération et messages. La connexion renvoie
`ACCOUNT_DELETED`. L'adresse privée reste réservée 14 jours et l'activité est conservée 30 jours. Le nettoyage
planifié traite les échéances par lots bornés. Les paquets publiés restent téléchargeables.

Les administrateurs consultent les échéances avec `GET /api/tokens/:name/retention`, libèrent l'adresse par
`DELETE /api/tokens/:name/retention/email` et effacent l'activité par
`DELETE /api/tokens/:name/retention/audit`. Les champs `deleted_at`, `email_release_at`,
`email_released_at`, `audit_purge_at` et `audit_purged_at` utilisent des millisecondes Unix.
La route administrateur `DELETE /api/tokens/:name` applique les mêmes conditions de fermeture définitive.
