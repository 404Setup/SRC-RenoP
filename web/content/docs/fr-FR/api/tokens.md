---
title: API Token et utilisateurs
order: 3
category: Référence API
description: Cycle de vie des API Token, limites d’authentification et administration des comptes
---

# API Token et utilisateurs

Un API Token est un identifiant machine durable appartenant à un compte. RenoP ne stocke que le condensat de recherche
SHA-256 du secret aléatoire de 256 bits. Le secret est renvoyé une seule fois lors de la création et ne peut pas être
récupéré ensuite.

Chaque requête doit satisfaire deux contrôles indépendants :

- le Token contient la capacité exigée par la route ;
- le compte propriétaire possède encore le droit d’effectuer l’opération sur la cible.

Une modification de rôle, de dépôt ou d’équipe prend donc effet sans recréer les Token.

## Gérer ses API Token

Les routes de gestion exigent le cookie navigateur HttpOnly `renop_session`. Un API Token, un mot de passe,
`Authorization: Session` ou un paramètre d’URL ne peut pas gérer les secrets.

### Lister les scopes attribuables

`GET /api/auth/profile/api-tokens/scopes`

La réponse dépend des droits actuels du compte. Les scopes administrateur ne sont jamais proposés à un compte ordinaire.

```json
{
  "scopes": ["repository:read", "repository:publish", "package:metadata"],
  "target_kinds": {
    "repository:read": "repository",
    "repository:publish": "repository",
    "package:metadata": "package"
  },
  "target_limit": 128
}
```

### Créer un Token

`POST /api/auth/profile/api-tokens`

```json
{
  "name": "CI publishing",
  "scopes": ["repository:read", "repository:publish"],
  "targets": {
    "repository:publish": ["releases"]
  },
  "expires_at": 1798761600000
}
```

`expires_at` est facultatif, en millisecondes Unix, entre cinq minutes et cinq ans après la création. Une valeur absente
ou nulle désactive l’expiration du Token. Un compte possède au plus 50 API Token.

`targets` limite indépendamment chaque scope. Un scope absent de `targets` s’applique à toutes les cibles encore
autorisées au compte. Une cible de dépôt est son nom exact. Une cible de paquet est `repository/package`; pour Maven,
utilisez par exemple `maven-releases/com.example/library`. Une cible d’équipe est `package/repository/package` ou
`domain/example.com`. Une cible de domaine est son nom canonique. La requête accepte au plus 128 cibles.

Les restrictions de cible ne remplacent jamais les droits du dépôt ni les niveaux L0-L4 actuels.

Une création réussie renvoie `201 Created` et `Cache-Control: no-store` :

```json
{
  "token": {
    "id": "07cdcf2e-0828-4a29-9817-cf771cc9fb0a",
    "name": "CI publishing",
    "scopes": ["repository:publish", "repository:read"],
    "targets": {"repository:publish": ["releases"]},
    "created_at": 1787731200000,
    "expires_at": 1798761600000
  },
  "secret": "rnp_pat_EXAMPLE_REDACTED_COPY_THE_REAL_VALUE_ONCE"
}
```

### Lister les métadonnées

`GET /api/auth/profile/api-tokens` renvoie uniquement les métadonnées non secrètes et la limite du compte.

### Révoquer un Token

`DELETE /api/auth/profile/api-tokens/{token_id}` renvoie `204 No Content` et invalide immédiatement le cache
d’authentification.

## Référence des scopes

| Scope | Capacité |
|:------|:---------|
| `repository:read` | Lire catalogues, métadonnées, fichiers, images et versions |
| `repository:publish` | Publier via Maven, npm, Cargo, Docker, files ou téléversement découpé |
| `repository:delete` | Supprimer fichiers, versions, tags ou images |
| `package:create` | Réserver un paquet npm/Cargo ou une image Docker après contrôle du dépôt |
| `package:metadata` | Modifier la description et les métadonnées d’un paquet |
| `package:lifecycle` | Archiver, restaurer, yank ou unyank un paquet ou une version |
| `team:manage` | Consulter et gérer les équipes et invitations npm, Cargo, Docker et domaines Maven |
| `domain:read` | Lire la configuration privée des domaines Maven |
| `domain:create` | Créer un domaine Maven |
| `domain:verify` | Vérifier ou forcer la vérification d’un domaine Maven |
| `domain:delete` | Supprimer un domaine Maven |
| `messages:read` | Lire, marquer et supprimer les messages du compte |
| `account:read` | Lire les données privées et le journal personnel |
| `account:write` | Modifier le profil public via l’API |
| `statistics:read` | Interroger les statistiques de téléchargement accessibles au compte |
| `admin:users` | Administrer les comptes et leurs appareils de connexion |
| `admin:repositories` | Administrer les dépôts et reconstruire les index |
| `admin:settings` | Administrer les paramètres et diagnostics |
| `admin:audit` | Lire ou nettoyer l’audit et l’état réservés aux administrateurs |
| `admin:notifications` | Composer des notifications administrateur |
| `admin:updates` | Vérifier, envoyer, installer et redémarrer les mises à jour |
| `admin:statistics` | Interroger les statistiques globales |

Seul un administrateur peut créer un scope `admin:*`, et celui-ci cesse d’autoriser l’opération si le compte perd ce
rôle. Les anciens scopes `package:manage` et `domain:manage` restent acceptés pour les Token existants mais ne sont plus
attribuables.

## Utiliser un Token

Pour une API de gestion autorisée, utilisez Bearer :

```http
Authorization: Bearer rnp_pat_REDACTED
```

Les clients de paquets peuvent employer le même Token comme mot de passe Basic avec le nom du compte. Basic Auth reste
limité aux protocoles de paquets. npm envoie le Token avec `_authToken` ou Basic ; Cargo l’envoie comme valeur complète de `Authorization`. Docker l’échange via
`/v2/token`; le jeton court ne contient que les actions permises par scopes et droits de l’image.

## Compatibilité

L’administration des utilisateurs reste sous `/api/tokens`, mais un administrateur ne peut pas créer un identifiant
pour un autre compte. L’ancien `POST /api/auth/profile/token` crée encore un Token de publication supplémentaire, sans
expiration, pour le compte connecté. Les nouvelles intégrations doivent utiliser les routes fines du profil.
