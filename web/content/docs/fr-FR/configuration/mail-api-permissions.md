---
title: Permissions des API de messagerie
order: 7
category: Configuration
description: Autorisations des fournisseurs, contrats API vérifiés et conditions de connexion
---

# Permissions des API de messagerie

Ce guide couvre les huit fournisseurs API de [Distribution des e-mails](mail.md), ainsi que SMTP OAuth.
Les adresses et noms de permissions ont été vérifiés dans les références officielles indiquées le 2026-09-08.
RenoP utilise directement HTTP ; aucun SDK de fournisseur n'est nécessaire.

## Tableau des permissions

Accordez l'envoi à l'identité configurée, puis ajoutez les permissions de consultation des fonctions utilisées.
L'acceptation d'une demande ne prouve pas sa livraison. Les permissions de lecture d'une boîte peuvent aussi donner
accès au contenu de ses messages.

| Fournisseur         | Envoi                                        | État                                                      | Quota / solde                                                                         |
|---------------------|----------------------------------------------|-----------------------------------------------------------|---------------------------------------------------------------------------------------|
| Cloudflare          | Email Sending: Edit limité au compte         | Réponse initiale                                          | Aucune consultation correspondante dans RenoP                                         |
| Microsoft Graph     | `Mail.Send`                                  | `Mail.Read` pour les propriétés étendues                  | Aucune consultation correspondante                                                    |
| Amazon SES v2       | `ses:SendEmail`                              | `ses:GetMessageInsights`                                  | `ses:GetAccount` ; aucun solde                                                        |
| SendGrid            | `mail.send`                                  | `messages.read` et supplément d'historique Email Activity | `user.credits.read` ; aucun solde monétaire                                           |
| Gmail               | `https://www.googleapis.com/auth/gmail.send` | `https://www.googleapis.com/auth/gmail.metadata`          | Aucun quota d'envoi ni solde                                                          |
| Alibaba Direct Mail | `dm:SingleSendMail`                          | `dm:SenderStatisticsDetailByParam`                        | `dm:DescAccountSummary` ; facultativement `bss:DescribeAcccount`                      |
| Tencent SES         | `ses:SendEmail`                              | `ses:GetSendEmailStatus`                                  | Facultativement `finance:DescribeAccountBalance` ; aucun quota d'envoi pris en charge |
| Feishu / Lark       | `mail:user_mailbox.message:send`             | `mail:user_mailbox.message:readonly`                      | Aucune consultation correspondante                                                    |

Les comptes OAuth nécessitent une autorisation initiale de l'utilisateur et un jeton de renouvellement pour le
renouvellement automatique.
RenoP renouvelle les identifiants enregistrés ; les paramètres de messagerie ne proposent pas de callback OAuth
interactif.
Utilisez le flux de code d'autorisation de votre application et son URI de redirection exacte, puis conservez le jeton
obtenu de manière privée.

## Cloudflare Email

Activez Email Sending pour le compte et configurez le domaine d'envoi avec les enregistrements DNS requis.
Créez un jeton **Email Sending: Edit** limité à ce compte, renseignez `api_key` et son `account_id`.
Les permissions Email Routing ne permettent pas l'envoi sortant. La gestion du domaine et du DNS est distincte des
droits nécessaires à l'exécution.

RenoP appelle `POST /accounts/{account_id}/email/sending/send` sous `https://api.cloudflare.com/client/v4`.
Il transmet des adresses structurées, du texte et du HTML, puis lit `message_id`, `delivered`, `permanent_bounces`,
`queued` et `suppressed_recipients`.
Un résultat en attente reste `queued_provider` ; RenoP ne consomme pas les abonnements aux événements distincts de
Cloudflare.
Consultez
la [configuration et les permissions](https://developers.cloudflare.com/email-service/get-started/send-emails/) et
le [schéma d'envoi](https://developers.cloudflare.com/api/resources/email_sending/methods/send/).

## Microsoft Graph

Inscrivez une application Entra dans le cloud de la boîte. Les comptes personnels Outlook.com utilisent une autorisation
déléguée ;
les organisations Microsoft 365 peuvent utiliser des permissions déléguées ou d'application.
En mode délégué, autorisez `Mail.Send Mail.Read offline_access`, enregistrez `client_id`, le `client_secret` applicable
et `refresh_token`, puis indiquez `mailbox: me`.
Le tenant `common` convient à une inscription globale compatible ; les politiques de l'organisation peuvent exiger
l'accord de l'administrateur.

L'accès applicatif exige les permissions d'application `Mail.Send` et `Mail.Read`, avec consentement de
l'administrateur.
Indiquez l'ID réel du tenant, l'ID client, le secret et l'ID utilisateur ou l'adresse de la `mailbox` cible.
RenoP demande `client_credentials` avec la portée `/.default` de la ressource Graph choisie.
Les jetons applicatifs ne peuvent pas utiliser `/me`. Limitez les boîtes accessibles avec les contrôles d'accès
applicatif pris en charge par Exchange.

| Cloud                                    | Base API                                       | Autorité des jetons                 |
|------------------------------------------|------------------------------------------------|-------------------------------------|
| Global / Outlook.com / Microsoft 365 GCC | `https://graph.microsoft.com/v1.0`             | `https://login.microsoftonline.com` |
| Gouvernement américain L4 / GCC High     | `https://graph.microsoft.us/v1.0`              | `https://login.microsoftonline.us`  |
| Gouvernement américain L5 / DoD          | `https://dod-graph.microsoft.us/v1.0`          | `https://login.microsoftonline.us`  |
| Chine / 21Vianet                         | `https://microsoftgraph.chinacloudapi.cn/v1.0` | `https://login.chinacloudapi.cn`    |

L'envoi utilise `POST /me/sendMail` ou `POST /users/{mailbox}/sendMail`, l'adresse `from` configurée et une copie dans
les éléments envoyés.
La consultation filtre les éléments envoyés par la propriété de suivi étendue de RenoP ; `Mail.ReadBasic` ne couvre pas
cette propriété.
Pour envoyer depuis une autre boîte en mode délégué, obtenez `Mail.Send.Shared`, la lecture partagée appropriée et Send
As/Send on Behalf dans Exchange ; l'accès direct à cette boîte exige aussi Full Access.
HTTP 202 signifie accepté ; trouver la copie enregistrée signifie seulement `sent`.
Voir [sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail), [permissions des propriétés étendues](https://learn.microsoft.com/en-us/graph/api/singlevaluelegacyextendedproperty-get?view=graph-rest-1.0),
[envoi partagé](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user)
et [adresses des clouds](https://learn.microsoft.com/en-us/graph/deployments).

## Amazon SES

Vérifiez l'identité de l'expéditeur dans la région sélectionnée et obtenez l'accès de production pour sortir du bac à
sable SES.
Configurez une clé IAM et son secret ; des identifiants temporaires nécessitent aussi leur jeton de session, à
renouveler avant expiration.
Les identifiants SMTP SES ne sont pas des identifiants API SES.

Accordez `ses:SendEmail` sur les ressources d'identité autorisées. Accordez `ses:GetAccount` et `ses:GetMessageInsights`
sur `*`, car ces consultations ne permettent pas une restriction par identité.
Les informations détaillées sur les messages nécessitent la fonction Virtual Deliverability Manager correspondante,
facturée séparément de l'envoi.
RenoP signe avec AWS SigV4, le service `ses` et la région configurée.

`POST /v2/email/outbound-emails` renvoie `MessageId` ; `GET /v2/email/account` fournit `SendingEnabled` et le quota sur
24 heures glissantes.
`GET /v2/email/insights/{message_id}/` fournit les événements par destinataire. Les limites strictes SES s'appliquent
même en mode d'envoi forcé.
Voir [actions IAM](https://docs.aws.amazon.com/service-authorization/latest/reference/list_sesv2.html),
[consultation du compte](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetAccount.html),
[informations des messages](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetMessageInsights.html)
et [adresses régionales](https://docs.aws.amazon.com/general/latest/gr/ses.html).

## Twilio SendGrid

Authentifiez l'expéditeur ou le domaine, puis créez une clé restreinte avec `mail.send`, `messages.read` et
`user.credits.read` selon vos besoins.
L'API Email Activity nécessite l'achat de l'historique supplémentaire ; une clé ou une offre limitée à l'envoi ne suffit
pas.
L'envoi européen exige une offre Pro ou supérieure admissible, un sous-utilisateur européen et une IP européenne ;
configurez sa clé dans le préréglage EU.

RenoP appelle `POST /mail/send`, `GET /messages` et `GET /user/credits` sous la base `/v3` sélectionnée.
Le `X-Message-Id` initial est un préfixe des identifiants d'activité par destinataire ; RenoP vérifie aussi l'adresse
exacte du destinataire.
Les valeurs des recherches utilisent des guillemets doubles, par exemple `msg_id LIKE "submission-id%"`.
Les crédits correspondent à un nombre d'e-mails, pas à un solde retirable.
Voir [portées des clés](https://www.twilio.com/docs/sendgrid/api-reference/api-key-permissions),
[accès à l'activité](https://www.twilio.com/docs/sendgrid/api-reference/email-activity/filter-all-messages),
[syntaxe des recherches](https://www.twilio.com/docs/sendgrid/for-developers/sending-email/getting-started-email-activity-api),
[crédits](https://www.twilio.com/docs/sendgrid/api-reference/users-api/retrieve-your-credit-balance)
et [envoi régional](https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send).

## Google Gmail

Activez l'API Gmail dans le projet Google Cloud du client OAuth et configurez l'écran de consentement.
Faites autoriser `https://www.googleapis.com/auth/gmail.send` et `https://www.googleapis.com/auth/gmail.metadata` par le
titulaire de la boîte.
Demandez `access_type=offline` ; obtenez un nouveau consentement si aucun jeton de renouvellement initial n'est délivré.
Une application externe conservée en Testing peut recevoir des jetons de renouvellement expirant après sept jours ; les
conditions de publication et de vérification dépendent de l'application.

Enregistrez l'ID client, le secret et le jeton de renouvellement. RenoP échange ce dernier sur
`https://oauth2.googleapis.com/token`.
L'expéditeur configuré doit être la boîte autorisée ou un alias d'envoi autorisé.
Ce connecteur n'implémente ni les clés JSON de compte de service, ni la génération JWT de délégation à l'échelle du
domaine.

`POST /users/me/messages/send` sous `https://gmail.googleapis.com/gmail/v1` reçoit du MIME base64url et renvoie un ID de
message.
RenoP consulte `GET /users/me/messages/{message_id}` avec `format=minimal` pour vérifier le libellé `SENT`.
Cela confirme `sent`, pas la livraison. Les quotas de requêtes API sont distincts des limites d'envoi de la boîte.
Voir [portées d'envoi](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send),
[accès aux métadonnées](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/get)
et [configuration OAuth](https://developers.google.com/identity/protocols/oauth2/web-server).

## Alibaba Cloud Direct Mail

Activez Direct Mail, vérifiez le domaine et configurez l'expéditeur dans la région choisie.
Utilisez une clé RAM avec `dm:SingleSendMail`, `dm:DescAccountSummary` et `dm:SenderStatisticsDetailByParam`, sur la
ressource `*`.
Pour le recalage facultatif du solde, accordez **`bss:DescribeAcccount`** : les trois `c` consécutifs font partie du nom
officiel.
L'action API reste `QueryAccountBalance`, version `2017-12-14` ; il ne s'agit pas d'une permission `dm:`.

RenoP utilise des requêtes RPC POST signées, version Direct Mail `2015-11-23`.
`SingleSendMail` renvoie `EnvId` ; `DescAccountSummary` expose les quotas gratuits et l'état du compte.
Les statistiques publiques n'ont pas d'ID individuel fiable ; RenoP renvoie donc `unknown` après consultation sans
attribuer un résultat à partir du seul destinataire.
Limitez le nom affiché de l'expéditeur à 15 caractères. Choisissez le service de facturation selon la région commerciale
du compte, indépendamment de la région d'envoi, et alignez la devise sur celle renvoyée.

Voir [envoi et limites](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-singlesendmail),
[permission de quota](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-descaccountsummary),
[statistiques](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-senderstatisticsdetailbyparam),
[autorisation du solde](https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-queryaccountbalance)
et [adresses](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint).

## Tencent Cloud SES

Vérifiez le domaine et l'expéditeur, puis accordez `ses:SendEmail` et `ses:GetSendEmailStatus` dans CAM.
La politique CAM officielle utilise la ressource `*` pour ces deux opérations.
La consultation facultative du solde exige `finance:DescribeAccountBalance`, bien que le service API signé s'appelle
`billing`.
Placez SecretId dans `api_key`, SecretKey dans `api_secret`, et le Token temporaire dans `session_token`, le cas
échéant.

Les comptes ordinaires doivent utiliser un modèle approuvé. Renseignez son ID positif dans `tencent_template_id`.
Créez un modèle HTML avec les variables textuelles suivantes, soumettez-le au fournisseur, puis utilisez son ID après
approbation :

```html
<div style="padding:24px;background:#f3f5f8;font-family:Arial,sans-serif">
  <div style="max-width:600px;margin:auto;padding:24px;background:white;border:1px solid #dfe5ef;border-radius:20px">
    <p style="color:#3158c9;font-weight:bold">RenoP</p>
    <h1>{{subject}}</h1>
    <div style="white-space:pre-wrap;overflow-wrap:anywhere">{{text}}</div>
  </div>
</div>
```

RenoP fournit le sujet et la notification complète en texte brut, en échappant les caractères HTML avant substitution.
Placez les variables dans le texte, jamais dans les attributs ou scripts. Le JSON `TemplateData` entier est limité à 800
octets UTF-8 ; les notifications plus longues échouent avant soumission, sans troncature.
Cet exemple ne garantit pas l'approbation du fournisseur. En mode modèle, le HTML approuvé détermine la présentation.
Un ID nul conserve `Simple` pour les anciens comptes disposant d'une autorisation spéciale de contenu personnalisé ; il
ne donne pas cette permission aux comptes ordinaires.

`SendEmail` et `GetSendEmailStatus` utilisent la version `2020-10-02` et la signature TC3 du service `ses`.
La consultation rapproche l'ID et le destinataire et distingue acceptation, livraison, rejet et report temporaire.
`DescribeAccountBalance` utilise la version `2018-07-09` ; RenoP convertit les centimes disponibles en millionièmes de
devise.
Les adresses chinoises et internationales correspondent à des systèmes de comptes et devises distincts.
Voir [SendEmail](https://intl.cloud.tencent.com/document/product/1084/39408),
[champs des modèles et états](https://intl.cloud.tencent.com/document/product/1084/39418),
[actions SES CAM](https://intl.cloud.tencent.com/document/product/598/57150),
[permissions de facturation](https://cloud.tencent.com/document/product/555/61542)
et [API de solde](https://cloud.tencent.com/document/api/555/20253).

## Feishu et Lark Mail

Créez et publiez une application personnalisée dans une organisation disposant d'une boîte Mail active.
Accordez `mail:user_mailbox.message:send`, `mail:user_mailbox.message:readonly` et `offline_access`.
Rendez l'application accessible à l'expéditeur, activez le renouvellement si les paramètres de sécurité proposent ce
commutateur, publiez les réglages et obtenez son consentement.
Utilisez un jeton **utilisateur** ; un jeton de tenant n'autorise pas les opérations d'envoi et de suivi utilisées ici.

Configurez l'ID de l'application, son secret, le jeton de renouvellement et l'adresse de la boîte ou `me`.
Feishu et Lark ont des inscriptions, identifiants et bases API distincts :
`https://open.feishu.cn/open-apis` et `https://open.larksuite.com/open-apis`.
RenoP renouvelle actuellement les jetons via l'interface compatible `POST /authen/v2/oauth/token` et persiste chaque
jeton renouvelé avant envoi.

`POST /mail/v1/user_mailboxes/{mailbox}/messages/send` reçoit du MIME base64url avec remplissage et renvoie
`data.message_id`.
`GET /mail/v1/user_mailboxes/{mailbox}/messages/{message_id}/send_status` renvoie les détails par destinataire :
4 signifie livré ; 3 et 6 signalent un échec ; 0, 1, 2 et 5 poursuivent la consultation.
Seuls les résultats de livraison par destinataire permettent d'établir l'état livré.
Voir [envoi](https://open.feishu.cn/document/server-docs/mail-v1/user_mailbox-message/send),
[état de livraison](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status),
[état dans Lark](https://open.larksuite.com/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status)
et [renouvellement](https://open.feishu.cn/document/authentication-management/access-token/refresh-user-access-token).

## SMTP OAuth

Pour Gmail SMTP, autorisez `https://mail.google.com/` ; la portée d'envoi seule de l'API Gmail ne suffit pas.
Pour Outlook.com/Microsoft 365 SMTP, autorisez `https://outlook.office.com/SMTP.Send` et `offline_access`.
Activez aussi SMTP AUTH lorsque les politiques de l'organisation ou de la boîte Microsoft 365 l'exigent.
Le nom d'utilisateur SMTP est l'adresse de la boîte ; configurez les identifiants client et de renouvellement
correspondants.

Le renouvellement SMTP automatique utilise des identifiants délégués, sans acquérir de jetons applicatifs
`SMTP.SendAsApp`.
Un jeton d'accès géré à l'extérieur peut être configuré, mais son renouvellement reste à la charge de l'administrateur.
Voir [Google SASL OAuth](https://developers.google.com/workspace/gmail/imap/xoauth2-protocol)
et [Microsoft SMTP OAuth](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth).

## Vérification et exploitation

Enregistrez le compte, envoyez un test depuis l'administration et vérifiez l'état final du travail ainsi que le résultat
du recalage.
Confirmez la réception dans une boîte que vous contrôlez. Une réponse 202 de mise en file prouve seulement l'acceptation
par la file.
Une permission de suivi absente peut aboutir à `unknown` après acceptation ; ne renvoyez pas un message pour ce seul
motif.
Examinez le code du fournisseur dans les journaux globaux et accordez uniquement la permission manquante.

L'examen des références et les tests HTTP locaux ne prouvent pas la validité de vos identifiants, abonnement, domaine ou
compte régional.
Cette vérification n'a pas utilisé d'identifiants réels de fournisseur. L'envoi, les quotas, le solde et la livraison
doivent être vérifiés avec les comptes du déploiement.
Les limites inconnues suivent [Distribution des e-mails](mail.md) ; les limites connues sont conservées si le recalage
échoue.
