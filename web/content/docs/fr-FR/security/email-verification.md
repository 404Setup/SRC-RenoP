---
title: Vérification de l’e-mail de sécurité
order: 5
category: Sécurité
description: Vérifier l’adresse privée du compte par code e-mail ou via GitHub
---

# Vérification de l’e-mail de sécurité

## Modifier une adresse

Modifiez l’adresse privée dans **Sécurité du compte**. Lorsque l’envoi d’e-mails est activé, l’enregistrement place un
message de vérification en file pour la nouvelle adresse. Saisissez son code à huit chiffres pour appliquer le
changement. L’adresse actuelle reste utilisable jusqu’à la réussite de la vérification. Lorsque l’envoi est désactivé,
l’adresse est enregistrée directement.

La liste noire ou blanche configurée s’applique aux deux méthodes, même lorsque l’envoi est désactivé. Les règles
acceptent une adresse complète, un @domaine exact ou un .suffixe incluant les sous-domaines. Le mode liste noire peut
utiliser la liste intégrée des e-mails temporaires ; le mode liste blanche l’ignore. Les formes Unicode et Punycode d’un
même domaine internationalisé correspondent à la même règle. Une adresse appartenant à un autre compte ou encore
réservée par un compte fermé ne peut pas être utilisée.

## Alias de connexion

Les adresses principales et secondaires partagent un registre unique de propriété. La liaison échoue si l’identité
externe ou l’une de ses adresses de contact appartient à un autre compte, y compris pendant la rétention après
fermeture. Liaison, réservation des adresses et changements de sécurité sont validés ensemble ; un échec ne modifie pas
le compte existant. Les adresses ne fusionnent jamais des comptes.

**Autres adresses de connexion** affiche les adresses secondaires privées. Elles identifient le même compte pour la
connexion par mot de passe ou Passkey, la réinitialisation par e-mail et les codes de récupération hors ligne ; les
politiques de mot de passe et de second facteur restent applicables. Les notifications utilisent toujours l’adresse
principale. Un compte peut conserver 128 adresses au maximum, adresse principale comprise.

Une adresse fournie doit être vérifiée par le fournisseur ou déjà appartenir au compte connecté. Sinon, vérifiez-la
auprès du fournisseur ou utilisez **Ajouter et vérifier une adresse** avant la liaison. L’ajout nécessite l’envoi
d’e-mails RenoP et le dialogue de code existant. Lors d’une inscription avec une adresse externe non vérifiée, il faut
confirmer cette adresse ; confirmer une autre adresse ne suffit pas. Les adresses GitHub no-reply sont exclues.

Déconnecter un fournisseur conserve ses alias. Supprimer un alias libère l’adresse ; une autorisation ultérieure peut la
réajouter. L’adresse principale ne peut pas être supprimée comme alias. Une suspension conserve toutes les adresses, et
la fermeture les réserve toutes pendant 14 jours, même sans adresse principale. La libération anticipée par un
administrateur concerne l’ensemble des adresses conservées.

| Méthode | Chemin                          | Requête ou réponse                                                                |
|---------|---------------------------------|-----------------------------------------------------------------------------------|
| GET     | `/api/auth/profile/security`    | Ajoute `email_aliases` à côté de `email`                                          |
| PUT     | `/api/auth/profile/email`       | `{"email":"alias@example.com","alias":true}` ; `202` avec un reçu de vérification |
| DELETE  | `/api/auth/profile/email/alias` | `{"email":"alias@example.com"}` ; sécurité du compte mise à jour                  |

La confirmation utilise l’endpoint existant ; son but est enregistré avec le défi et ne peut pas être changé dans la
requête de confirmation. La suppression exige une connexion navigateur datant de moins de cinq minutes. Les erreurs
`ACCOUNT_EMAIL_PROOF_REQUIRED`, `ACCOUNT_EMAIL_LIMIT` et `ACCOUNT_EMAIL_PRIMARY` renvoient `409` ; une session trop
ancienne reçoit `MFA_REAUTH_REQUIRED`.

La mise à niveau renseigne la propriété des adresses principales existantes. Les anciennes liaisons acquièrent leurs
alias à la prochaine autorisation, car ces adresses n’étaient pas conservées auparavant. La persistance revérifie
suspension et expiration du compte dans sa transaction avant de créer une nouvelle session de navigateur.

## API de vérification

Ces opérations exigent le cookie du navigateur actuel. Les corps JSON doivent utiliser `Content-Type: application/json`
et sont limités à 4 096 octets.

| Méthode | Chemin                            | Requête ou réponse                                                                                                         |
|---------|-----------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| GET     | `/api/auth/profile/security`      | Inclut `email` et `email_verification_required`                                                                            |
| PUT     | `/api/auth/profile/email`         | `{"email":"new@example.com"}` ; `202` avec `{id,status,ticket}` après mise en file, sinon `200` avec la sécurité du compte |
| POST    | `/api/auth/profile/email/confirm` | Adresse et code ; `200` avec la sécurité du compte mise à jour                                                             |
| GET     | `/api/auth/mail/:id`              | État de livraison ; accès autorisé par le compte demandeur ou `X-Renop-Mail-Ticket`                                        |

```json
{"email":"new@example.com","code":"01234567"}
```

Les codes expirent après 10 minutes, autorisent cinq tentatives incorrectes et sont liés à la session du navigateur
demandeur. Un nouveau code peut être demandé après 60 secondes, sous réserve de la limite d’envoi par IP. La mise en
file réussie d’un remplacement invalide l’ancien code ; un échec préserve celui-ci. Le stockage du code, la mise en file
et le compteur IP sont validés dans une même transaction.

Un code incorrect, expiré, consommé ou devenu obsolète renvoie `400` avec `ACCOUNT_EMAIL_CODE_INVALID` ; le navigateur
reste connecté. Une adresse occupée renvoie `409` avec `ACCOUNT_EMAIL_CONFLICT`. Un refus par la politique des
destinataires renvoie `400` avec `mail_recipient_blocked`. Une session révoquée ne peut pas confirmer une modification.
Un changement des identifiants ou des paramètres de sécurité invalide les preuves en attente.

## Vérification GitHub

Lorsque la connexion GitHub est configurée, **Utiliser l’e-mail vérifié de GitHub** autorise une nouvelle consultation
via `GET /api/auth/github/start?intent=email`. Ce parcours demande `user:email` et consulte
la [liste des e-mails de l’utilisateur GitHub authentifié](https://docs.github.com/en/rest/users/emails#list-email-addresses-for-the-authenticated-user).

RenoP choisit l’adresse de contact principale vérifiée, ou la première adresse de contact vérifiée en l’absence
d’adresse principale. Les adresses no-reply de GitHub sont exclues. L’adresse obtenue est contrôlée par la politique
locale puis enregistrée. Cette méthode fonctionne sans expéditeur configuré et ne crée ni ne remplace une liaison de
connexion GitHub.

Le callback est à usage unique, expire après 10 minutes et doit revenir au compte et à la session d’origine avec des
identifiants inchangés. Les jetons d’accès du fournisseur servent uniquement à cette consultation et ne sont pas
conservés.

## Exploitation

RenoP conserve au maximum 2 048 modifications d’e-mail en attente. Les enregistrements expirés sont supprimés avant
insertion et lors du nettoyage du courrier. Les codes sont stockés sous forme de hachages avec clé ; le contenu des
messages en file est chiffré avec la clé privée du courrier. Les codes et tickets restent exclus des URL, journaux et
stockages du navigateur.

Une modification réussie produit une entrée d’activité du profil. Le worker de courrier séquentiel envoie une
notification `email_changed` selon le routage et les limites configurés. La mise en file est distincte de la livraison ;
la boîte de vérification affiche la progression et les échecs signalés par le fournisseur.

Les autres fournisseurs peuvent proposer [Utiliser l’e-mail vérifié](./oauth-login.md) dans l’éditeur de profil. Cette
opération exige aussi une nouvelle autorisation, conserve l’association actuelle et respecte la politique des
destinataires. Sans déclaration explicite de vérification, l’adresse doit être confirmée par un code RenoP.

## Langue du compte

La connexion restaure la langue enregistrée du compte sur chaque appareil. Un compte sans préférence adopte la langue
actuelle de l’interface ; les changements suivants dans le sélecteur sont enregistrés automatiquement. L’inscription
enregistre la langue de la page avec le compte. En cas d’échec, un choix en attente reste sur l’appareil et est renvoyé
à la reconnexion, au retour sur la page ou après son rechargement. Le choix d’un ancien compte n’est jamais appliqué à
un autre compte.

Cette préférence est privée, survit aux changements de nom et aux redémarrages, et est supprimée à la clôture du compte.
Sa modification ne change pas les identifiants et n’invalide pas une vérification à deux étapes en cours. Les
notifications automatiques et les e-mails de réinitialisation utilisent la préférence du destinataire, y compris quand
un alias identifie le compte. Sans préférence, ils utilisent l’en-tête `Accept-Language` de la page d’origine, puis
`en-US`. Les messages en file conservent la langue choisie lors de leur ajout.

Les identifiants disponibles sont `en-US`, `zh-CN`, `zh-HK`, `zh-TW`, `zh-YUE`, `ko-KR`, `ja-JP`, `de-DE`, `fr-FR`,
`ru-RU`, `es-ES` et `pt-PT`.

| Méthode | Chemin                     | Requête ou réponse                                                                                                      |
|---------|----------------------------|-------------------------------------------------------------------------------------------------------------------------|
| GET     | `/api/auth/profile/locale` | `{"user_id":"00000000-0000-4000-8000-000000000001","locale":"fr-FR"}` ; une valeur vide indique l’absence de préférence |
| PUT     | `/api/auth/profile/locale` | `{"user_id":"00000000-0000-4000-8000-000000000001","locale":"fr-FR"}` ; renvoie l’identifiant canonique enregistré      |

Les deux opérations exigent le cookie du navigateur actuel et renvoient `Cache-Control: no-store` pour les données de
préférence. Les jetons API ne peuvent ni lire ni modifier cette préférence. Une valeur non prise en charge renvoie `400`
avec `ACCOUNT_LOCALE_INVALID`. La [configuration des e-mails](../configuration/mail.md) ne propose plus de sélecteur de
langue global.

PUT doit reprendre le `user_id` immuable renvoyé par GET ; une autre identité reçoit `403`. Les choix locaux en attente
sont également liés à cet ID : un renommage ou une réutilisation ultérieure du nom ne transfère pas les préférences
entre comptes.
