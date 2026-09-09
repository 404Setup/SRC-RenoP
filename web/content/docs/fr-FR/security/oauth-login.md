---
title: Connexion via un service tiers
order: 7
category: Sécurité
description: Configurer Microsoft, Google, GitLab, Cloudflare, Stack Exchange et les fournisseurs OAuth personnalisés
---

# Connexion via un service tiers

## Configurer les fournisseurs

Dans les paramètres de service de l’administrateur, ouvrez **Connexion via un service tiers**, sélectionnez un
fournisseur et ajoutez un client. Renseignez ses identifiants et l’URL de rappel exacte, puis activez-le et enregistrez.
Chaque client possède un `id` unique en minuscules, de 32 caractères maximum : lettres, chiffres, traits de soulignement
et traits d’union, avec une lettre en premier. `github` est réservé. Cet ID identifie les associations existantes et ne
peut plus être modifié dans l’interface après enregistrement. Jusqu’à 32 fournisseurs et 32 associations par compte sont
autorisés. GitHub conserve sa section dédiée.

Enregistrez une application web chez le fournisseur et autorisez exactement le rappel
`https://renop.example/api/auth/oauth/<provider-id>/callback`. Les points de terminaison OAuth et les rappels exigent
HTTPS, sauf les adresses HTTP de bouclage utilisées en développement. Les nouvelles autorisations utilisent
immédiatement les paramètres enregistrés ; toute modification invalide les autorisations en cours.

| Préréglage      | Configuration de l’application et options                                                                                                                                                                                                                                                                                                                                                           | Traitement de l’e-mail                                                                                                                                 |
|-----------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| `microsoft`     | [Plateforme d’identités Microsoft](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc) ; `tenant` vaut `common`, `organizations`, `consumers` ou l’UUID d’un locataire Entra. Respectez les types de comptes pris en charge par l’application. Portées par défaut : `openid profile email`.                                                                                | [UserInfo](https://learn.microsoft.com/en-us/entra/identity-platform/userinfo) ne certifie pas la vérification de l’e-mail ; un code RenoP est requis. |
| `google`        | [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect) ; portées par défaut : `openid profile email`.                                                                                                                                                                                                                                                        | Une adresse n’est vérifiée que si `email_verified` est le booléen `true`.                                                                              |
| `gitlab`        | [GitLab OpenID Connect](https://docs.gitlab.com/integration/openid_connect_provider/) ; `base_url` vaut `https://gitlab.com` par défaut et peut désigner une instance auto-hébergée. Portées par défaut : `openid profile email`.                                                                                                                                                                   | Un code est requis si aucune adresse de contact vérifiée n’est renvoyée.                                                                               |
| `cloudflare`    | [Créez un client OAuth](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/) et [intégrez-le](https://developers.cloudflare.com/fundamentals/oauth/integrate-with-cloudflare/). La portée `openid` fournit le sujet stable. Les clients privés sont réservés aux membres du compte Cloudflare ; les clients publics nécessitent la vérification du domaine par Cloudflare. | Ce préréglage ne fournit pas d’e-mail vérifié ; un code RenoP est requis.                                                                              |
| `stackexchange` | Enregistrez une [application Stack Apps](https://stackapps.com/help/api-authentication), configurez ses identifiants et sa clé API, puis choisissez `site` (`stackoverflow` par défaut). RenoP suit le [flux de code d’autorisation](https://api.stackexchange.com/docs/authentication) avec PKCE et utilise l’`account_id` du réseau.                                                              | Aucune adresse de contact n’est fournie ; un code RenoP est requis.                                                                                    |
| `custom`        | Configurez les URL d’autorisation, de jetons et d’informations utilisateur, ainsi que les chemins des champs JSON. Ajoutez l’émetteur et JWKS pour OpenID Connect.                                                                                                                                                                                                                                  | L’adresse n’est fiable qu’avec un champ de vérification associé dont la valeur est le booléen `true` ; sinon, un code est requis.                      |

Microsoft prend en charge les comptes personnels et Entra ID selon le locataire et l’enregistrement de l’application.
Cloudflare et Stack Exchange n’exigent pas de correspondance d’e-mail fictive. Pour permettre à leurs utilisateurs de
s’inscrire, activez d’abord [l’envoi d’e-mails](../configuration/mail.md).

## Configuration et identifiants

Les fournisseurs sont enregistrés dans `server.oauth_providers`. Cet exemple laisse les clients désactivés jusqu’au
remplacement de leurs identifiants :

```yaml
server:
  oauth_providers:
    - id: microsoft
      type: microsoft
      name: Microsoft
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/microsoft/callback
      tenant: common
    - id: example
      type: custom
      name: Example Identity
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/example/callback
      authorize_url: https://identity.example/authorize
      token_url: https://identity.example/token
      userinfo_url: https://identity.example/userinfo
      scopes: ""
      token_auth: client_secret_post
      disable_pkce: false
      claims:
        subject: user.id
        username: user.username
        name: user.name
        email: user.email
        email_verified: user.email_verified
        avatar: user.picture
```

`name` accepte 80 octets UTF-8 ; `client_id`, 512 octets ; les secrets et clés API, 4 096 octets. `scopes` est une
chaîne séparée par des espaces, limitée à 1 024 octets. Les URL des points de terminaison acceptent 2 048 octets. Les
préréglages intégrés fournissent les points de terminaison officiels et les correspondances de champs ; utilisez
`custom` pour d’autres structures.

Les `claims` personnalisés sélectionnent des valeurs scalaires à l’aide de chemins JSON avec des points et d’indices
numériques, comme `user.id` ou `items.0.id`. Les chemins acceptent 128 caractères. `subject` doit être un identifiant
stable du fournisseur ; les noms et e-mails ne doivent pas identifier un compte. Les identifiants numériques conservent
leur précision. Les champs de profil facultatifs peuvent être omis. La chaîne `"true"` ne vérifie pas une adresse.

Pour un client personnalisé, `token_auth` accepte `client_secret_post`, `client_secret_basic` ou `none`. PKCE S256 est
activé par défaut. `disable_pkce: true` est réservé aux fournisseurs personnalisés dotés d’un secret client. Pour OpenID
Connect, configurez `issuer` et `jwks_url` et incluez `openid` dans `scopes` ; RenoP exige alors un jeton d’identité et
le vérifie. Les clients OAuth sans OIDC utilisent la réponse authentifiée d’informations utilisateur.

Les clients personnalisés utilisant uniquement OAuth incluent le chemin du champ d’identité dans leur autorité. Modifier
ce champ exige une nouvelle association. Si vous avez activé un tel client depuis le commit `26a1c1a`, réassociez-le
après cette mise à jour. Les clients OIDC utilisent toujours le sujet vérifié du jeton d’identité. Cloudflare ne fournit
que `sub` : les actions d’importation d’e-mail et de photo sont indisponibles ; l’inscription utilise des informations
saisies manuellement et un code e-mail RenoP.

Les lectures exposent `client_secret_configured` et `api_key_configured`, mais laissent les secrets vides. Une écriture
vide conserve un secret uniquement si l’ID, le type de fournisseur, l’ID client et le point de terminaison des jetons
sont identiques. `clear_client_secret` et `clear_api_key` les suppriment explicitement. Un changement de client ou de
point de terminaison exige de ressaisir les identifiants. Supprimer un fournisseur bloque les nouvelles autorisations
mais conserve les associations pour que les utilisateurs puissent les retirer.

## Inscription et gestion des comptes

La connexion d’une identité non associée lance [l’inscription](./registration.md). Aucun compte ni session n’existe
avant la confirmation et la définition d’un mot de passe. Tous les fournisseurs respectent le délai de dix minutes, le
délai d’attente après expiration, le quota par IP, la politique des destinataires, les limites locales des noms et
pseudonymes et les quotas d’avatars. Les comptes existants ne sont jamais sélectionnés ou fusionnés sur la seule base
d’une adresse e-mail identique.

La réponse en attente fournit `provider`, `provider_name`, `email_required`, `mail_enabled` et `avatar_available`.
Lorsque `email_required` vaut true, l’utilisateur doit saisir et vérifier une adresse, même si le fournisseur en suggère
une non vérifiée. Envoyez `provider` avec `email` au point de terminaison du code d’inscription. Conservez le même ID de
fournisseur pour la confirmation finale et joignez le `code` reçu. Un nouveau code ne prolonge pas le délai initial et
ne remet pas les tentatives à zéro. Sans service de courrier disponible, un fournisseur exigeant la vérification
d’e-mail ne peut pas terminer l’inscription.

Si le fournisseur renvoie une adresse vérifiée, celle-ci reste fixe durant la confirmation et aucun code n’est
nécessaire. L’importation du nom d’utilisateur, du pseudonyme et de l’avatar est facultative ; une photo absente ou trop
volumineuse n’annule pas l’inscription. Les noms restent soumis aux limites de l’instance et un nom déjà pris doit être
remplacé manuellement.

Dans l’éditeur de profil, associez ou actualisez un fournisseur, importez sa photo, utilisez son dernier e-mail vérifié
ou dissociez-le. La vérification d’e-mail exige une nouvelle autorisation et ne modifie pas l’association de connexion.
Les fournisseurs sans champ de vérification d’e-mail ne proposent pas cette action. L’importation d’une photo exige que
l’identité choisie soit liée au compte actuel. Le serveur conserve une autre méthode principale avant toute
dissociation. La [vérification en deux étapes](./two-step-verification.md), les suspensions et la fermeture définitive
s’appliquent aux connexions externes.

## API

| Méthode | Chemin                               | Contrat                                                                                |
|---------|--------------------------------------|----------------------------------------------------------------------------------------|
| GET     | `/api/auth/oauth/providers`          | Liste publique des ID et noms des fournisseurs configurés                              |
| GET     | `/api/auth/oauth/:provider/start`    | `intent=login`, `register`, `link`, `email` ou `avatar` ; `return_to` local facultatif |
| GET     | `/api/auth/oauth/:provider/callback` | Code d’autorisation à usage unique et état lié au navigateur                           |
| GET     | `/api/auth/profile/oauth`            | États privés, identifiant affiché, date d’autorisation et actions permises             |
| DELETE  | `/api/auth/profile/oauth/:provider`  | Dissociation : `204`, ou `409` avec `oauth_last_login_method`                          |
| GET     | `/api/settings/oauth-providers`      | Vue administrateur : `providers` et `presets`, sans secrets                            |
| PUT     | `/api/settings/oauth-providers`      | JSON administrateur `{providers:[...]}` remplaçant la liste ; corps limité à 128 KiB   |

Les opérations de profil exigent la session actuelle du navigateur. Le rappel renvoie un marqueur stable dans `oauth` et
l’ID dans `provider` ; la SPA traduit puis supprime ces paramètres. Les erreurs n’affichent jamais les réponses brutes
du fournisseur. La méthode de session est `oauth:<provider-id>`, éventuellement suivie de `+totp` ou `+passkey`.

## Autorisation Maven avec GitLab

Une autorisation GitLab.com datant de moins d’une heure peut vérifier automatiquement un nouveau domaine
`io.gitlab.<namespace>`. RenoP accepte l’espace personnel du compte et les groupes de premier niveau figurant dans la
déclaration de propriété GitLab. L’appartenance à un groupe public, la propriété d’un sous-groupe et les identités
GitLab auto-hébergées n’autorisent ni un espace parent ni un espace GitLab.com. Actualisez l’association pour renouveler
la preuve. Chaque identité conserve au maximum 1 001 espaces de noms. La vérification publique par biographie ou
description reste disponible.

## Sécurité et exploitation

Les rappels OAuth utilisent un cookie HttpOnly de dix minutes, un état serveur à usage unique et PKCE. Le processus
conserve au plus 2 048 états d’autorisation externe. Une modification de configuration invalide les rappels et
inscriptions en attente. OIDC vérifie la signature, l’émetteur, l’audience, le sujet, le nonce, les dates et le hachage
du jeton lorsqu’il est fourni ; RS256 et ES256 sont pris en charge. Les réponses sont limitées à 1 MiB et les requêtes
ont des délais bornés. Le proxy sortant configuré s’applique.

RenoP rattache les sujets stables à une autorité dérivée du type de fournisseur, du client, des points de terminaison et
de l’émetteur vérifié. Modifier cette autorité ne transfère pas les associations à un autre service d’identité. Les
jetons d’accès ne sont pas conservés pour la connexion. Une photo protégée peut nécessiter un jeton chiffré, conservé
uniquement dans l’inscription en attente jusqu’à confirmation ou expiration ; la clé privée `mfa_encryption_key` protège
cette valeur temporaire, qui n’est jamais renvoyée au navigateur.

La fermeture d’un compte libère atomiquement toutes les associations externes. Un autre compte actif peut les récupérer,
mais le nom d’utilisateur reste réservé définitivement, l’e-mail pendant 14 jours et l’activité pendant 30 jours. Un
rappel ou une actualisation tardive ne peut pas recréer les associations d’un compte fermé.

Une nouvelle liaison exige la disponibilité simultanée de l’identité externe et de toutes les adresses de contact
récupérées. Une identité libérée ne permet pas de contourner la propriété des adresses conservées par un autre compte.
Consultez les [alias de connexion](./email-verification.md) pour les règles de vérification, suppression et rétention.
