---
title: Inscription des comptes
order: 6
category: Sécurité
description: Activer l'inscription, la confirmation par courriel et la création de comptes GitHub
---

# Inscription des comptes

## Paramètres d'inscription

L'inscription est désactivée par défaut. Un administrateur peut l'activer dans les paramètres du service. Sa
désactivation fait retourner `404` à `/account/register` et aux opérations d'inscription ; le statut public reste
accessible. Voici les valeurs par défaut de la configuration de premier niveau :

```yaml
registration:
  enabled: false
  ip_limit: 1
  ip_interval: {value: 3, unit: week}
  provider_cooldown: {value: 12, unit: hour}
```

Seules les inscriptions réussies consomment le quota IP : un compte toutes les trois semaines par défaut, à compter de
la première inscription réussie. Les écritures IPv4 et IPv6 équivalentes partagent la limite. La fermeture du compte ne
restitue pas le quota ; les limites survivent aux redémarrages. Le nombre doit être compris entre 1 et 10 000 ; les
intervalles sont positifs, utilisent `minute`, `hour`, `day`, `week` ou `month`, et ne dépassent pas 365 jours. Un mois
vaut 30 jours.

## Créer un compte

Ouvrez l'inscription depuis la page de connexion. Le nom d'utilisateur contient 4 à 18 lettres ASCII, chiffres ou traits
de soulignement et est conservé en minuscules. Le pseudonyme facultatif accepte 36 caractères Unicode. Le mot de passe
est obligatoire et contient 6 à 72 octets UTF-8.

Lorsque l'envoi de courriels est activé, une adresse et son code à huit chiffres sont obligatoires. Le code expire après
dix minutes et autorise cinq erreurs. Confirmez avec le même navigateur et la même adresse IP. L'envoi utilise la file
série, la politique des destinataires, les quotas et les limites existants ; la page affiche son statut. Sans envoi de
courriels, l'adresse est facultative. Connectez-vous après l'inscription ; un message de réussite est mis en file
lorsque l'envoi est activé.

## S'inscrire avec GitHub

Une connexion GitHub sans compte associé ouvre la confirmation. RenoP demande `read:user read:org user:email`, choisit
une adresse de contact réelle vérifiée et exclut les adresses no-reply. Aucun code par courriel n'est nécessaire.
L'autorisation est liée au navigateur par un cookie HttpOnly de dix minutes ; chaque état de rappel est à usage unique.

Confirmez sous dix minutes et définissez un mot de passe. Aucun compte utilisable n'existe avant confirmation. À
expiration, les données personnelles en attente sont supprimées et la même identité GitHub doit attendre le délai
configuré, douze heures par défaut après expiration. L'import du nom, du pseudonyme et de l'avatar est facultatif. Les
limites locales s'appliquent ; un nom indisponible doit être saisi manuellement. Des données facultatives absentes ou un
avatar dépassant la taille ou le quota ne bloquent pas l'inscription.

## API d'inscription

Les requêtes JSON publiques exigent `Content-Type: application/json` et sont limitées à 4 096 octets. La confirmation
utilise aussi le cookie HttpOnly privé d'inscription ; ne conservez jamais mot de passe, code ou ticket de courriel dans
le stockage du navigateur.

- `GET /api/auth/registration/status`: Lire la disponibilité et les exigences de courriel.
- `GET /api/auth/registration/pending`: Lire la confirmation de ce navigateur.
- `POST /api/auth/registration/code`: Mettre un code en file ; retourne `202` avec un reçu privé.
- `POST /api/auth/registration`: Confirmer l'inscription.
- `GET /api/settings/registration`, `PUT /api/settings/registration`: Lire ou modifier les paramètres (administrateur).

```json
{
  "username": "new_user",
  "nickname": "New User",
  "email": "user@example.com",
  "password": "a unique long password",
  "code": "12345678"
}
```

Utilisez `provider: "github"` pour confirmer GitHub, conservez l'adresse vérifiée et omettez `code`. `import_avatar` à
`true` demande l'import de l'avatar. Le succès retourne `201`, `username` et `avatar_imported`, sans ouvrir de session.
Les conflits retournent `409`, une confirmation invalide ou expirée `400`, une limite IP atteinte ou un délai
fournisseur `429`. Compte, adresse, liaison, comptage IP et consommation de la confirmation sont validés dans une seule
transaction.

Les autres services suivent les [règles d’inscription externe](./oauth-login.md) : envoyez leur ID pour l’émission du
code comme pour la confirmation. Une adresse absente ou non vérifiée par le fournisseur exige toujours un code RenoP ;
cette inscription ne peut aboutir sans courrier disponible. Une adresse déjà vérifiée n’exige aucun code supplémentaire.
L’e-mail facultatif sans courrier concerne uniquement l’inscription manuelle.
