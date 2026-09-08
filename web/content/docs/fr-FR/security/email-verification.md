---
title: Vérification de l’e-mail de sécurité
order: 5
category: Sécurité
description: Vérifier l’adresse privée du compte par code e-mail ou via GitHub
---

# Vérification de l’e-mail de sécurité

## Modifier une adresse

Modifiez l’adresse privée dans **Sécurité du compte**. Lorsque l’envoi d’e-mails est activé, l’enregistrement place un message de vérification en file pour la nouvelle adresse. Saisissez son code à huit chiffres pour appliquer le changement. L’adresse actuelle reste utilisable jusqu’à la réussite de la vérification. Lorsque l’envoi est désactivé, l’adresse est enregistrée directement.

La liste noire ou blanche configurée s’applique aux deux méthodes, même lorsque l’envoi est désactivé. Les règles correspondent à une adresse exacte ou à un domaine, sans ses sous-domaines. Les formes Unicode et Punycode d’un même domaine internationalisé correspondent à la même règle. Une adresse appartenant à un autre compte ou encore réservée par un compte fermé ne peut pas être utilisée.

## API de vérification

Ces opérations exigent le cookie du navigateur actuel. Les corps JSON doivent utiliser `Content-Type: application/json` et sont limités à 4 096 octets.

| Méthode | Chemin | Requête ou réponse |
|---|---|---|
| GET | `/api/auth/profile/security` | Inclut `email` et `email_verification_required` |
| PUT | `/api/auth/profile/email` | `{"email":"new@example.com"}` ; `202` avec `{id,status,ticket}` après mise en file, sinon `200` avec la sécurité du compte |
| POST | `/api/auth/profile/email/confirm` | Adresse et code ; `200` avec la sécurité du compte mise à jour |
| GET | `/api/auth/mail/:id` | État de livraison ; accès autorisé par le compte demandeur ou `X-Renop-Mail-Ticket` |

```json
{"email":"new@example.com","code":"01234567"}
```

Les codes expirent après 10 minutes, autorisent cinq tentatives incorrectes et sont liés à la session du navigateur demandeur. Un nouveau code peut être demandé après 60 secondes, sous réserve de la limite d’envoi par IP. La mise en file réussie d’un remplacement invalide l’ancien code ; un échec préserve celui-ci. Le stockage du code, la mise en file et le compteur IP sont validés dans une même transaction.

Un code incorrect, expiré, consommé ou devenu obsolète renvoie `400` avec `ACCOUNT_EMAIL_CODE_INVALID` ; le navigateur reste connecté. Une adresse occupée renvoie `409` avec `ACCOUNT_EMAIL_CONFLICT`. Un refus par la politique des destinataires renvoie `400` avec `mail_recipient_blocked`. Une session révoquée ne peut pas confirmer une modification. Un changement des identifiants ou des paramètres de sécurité invalide les preuves en attente.

## Vérification GitHub

Lorsque la connexion GitHub est configurée, **Utiliser l’e-mail vérifié de GitHub** autorise une nouvelle consultation via `GET /api/auth/github/start?intent=email`. Ce parcours demande `user:email` et consulte la [liste des e-mails de l’utilisateur GitHub authentifié](https://docs.github.com/en/rest/users/emails#list-email-addresses-for-the-authenticated-user).

RenoP choisit l’adresse de contact principale vérifiée, ou la première adresse de contact vérifiée en l’absence d’adresse principale. Les adresses no-reply de GitHub sont exclues. L’adresse obtenue est contrôlée par la politique locale puis enregistrée. Cette méthode fonctionne sans expéditeur configuré et ne crée ni ne remplace une liaison de connexion GitHub.

Le callback est à usage unique, expire après 10 minutes et doit revenir au compte et à la session d’origine avec des identifiants inchangés. Les jetons d’accès du fournisseur servent uniquement à cette consultation et ne sont pas conservés.

## Exploitation

RenoP conserve au maximum 2 048 modifications d’e-mail en attente. Les enregistrements expirés sont supprimés avant insertion et lors du nettoyage du courrier. Les codes sont stockés sous forme de hachages avec clé ; le contenu des messages en file est chiffré avec la clé privée du courrier. Les codes et tickets restent exclus des URL, journaux et stockages du navigateur.

Une modification réussie produit une entrée d’activité du profil. Le worker de courrier séquentiel envoie une notification `email_changed` selon le routage et les limites configurés. La mise en file est distincte de la livraison ; la boîte de vérification affiche la progression et les échecs signalés par le fournisseur.

Les autres fournisseurs peuvent proposer [Utiliser l’e-mail vérifié](./oauth-login.md) dans l’éditeur de profil. Cette opération exige aussi une nouvelle autorisation, conserve l’association actuelle et respecte la politique des destinataires. Sans déclaration explicite de vérification, l’adresse doit être confirmée par un code RenoP.
