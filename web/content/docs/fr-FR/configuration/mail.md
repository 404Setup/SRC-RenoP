---
title: Envoi de courriels
order: 6
category: Configuration
description: Fournisseurs, file persistante, quotas, facturation, modèles et API d’administration
---

# Envoi de courriels

Configurez les courriels dans **Paramètres → Service** : ajoutez un compte, choisissez un préréglage, saisissez les identifiants et enregistrez.
Activez ensuite le service avec l’URL HTTPS publique de l’instance. Les changements s’appliquent aux opérations suivantes sans redémarrage.

## Configuration

```yaml
mail:
  enabled: false
  public_url: https://packages.example.com
  site_name: RenoP
  template_style: card
  delay: {value: 5, unit: second}
  manual_rate: {limit: 1, interval: {value: 2, unit: minute}}
  account_rate: {limit: 50, interval: {value: 1, unit: minute}}
  calibration: {value: 5, unit: minute}
  list_mode: blacklist
  use_disposable_blacklist: false
  addresses: []
  accounts:
    - id: primary
      name: Main mailbox
      enabled: true
      provider: smtp
      preset: smtp-custom
      scenes: ["*"]
      from: noreply@example.com
      from_name: RenoP
      smtp_host: smtp.example.com
      smtp_port: 587
      smtp_security: starttls
      username: noreply@example.com
      password: ""
      quota: {limit: 0, period: month}
      force_send: false
      overage: {limit: -1, period: month}
      balance_micros: null
      fetch_balance: false
      pricing:
        currency: USD
        rounding: proportional
        tiers:
          - {up_to: 0, amount_micros: 100000, batch_size: 1000}
```

Un seul compte actif prend en charge toutes les situations. Avec plusieurs comptes, les affectations explicites priment sur l’unique compte de repli `*`.
Deux comptes actifs ne peuvent pas revendiquer la même situation. Une situation sans affectation n’a pas d’expéditeur.
L’`id` du compte est stable : changer son nom conserve les compteurs. Supprimer un compte annule ses courriels non envoyés lors de leur prochain traitement.
Désactiver le service ou un compte suspend l’envoi, sans prolonger l’expiration des messages.

`delay` accepte secondes, minutes ou heures ; zéro supprime l’attente, mais les envois restent séquentiels.
`manual_rate` limite les demandes manuelles par IP, y compris les tests, sur une période en minutes, heures ou jours.
`account_rate` inclut les tentatives automatiques et manuelles par compte ; sa période accepte aussi les secondes.
Limites et durées doivent être positives. Valeurs par défaut : une demande manuelle par IP toutes les deux minutes et 50 tentatives par compte par minute.

`list_mode` accepte `blacklist` ou `whitelist`. La politique est vérifiée avant la mise en file et juste avant l’envoi. Les domaines internationalisés correspondent de la même manière en Unicode et Punycode ; le point final du domaine est ignoré.

| Règle | Correspondance |
|---|---|
| `person@example.com` | Cette adresse complète |
| `@example.com` | Ce domaine exact de fournisseur, sans sous-domaines |
| `.com` ou `.example.com` | Le suffixe DNS complet, domaine de base et sous-domaines compris |

Activez `use_disposable_blacklist: true` pour compléter les règles de liste noire avec la liste intégrée de messageries temporaires. L’option est désactivée par défaut et ignorée en mode liste blanche. L’instantané combine 75 627 domaines de [disposable/disposable-email-domains](https://github.com/disposable/disposable-email-domains) et [disposable-email-domains/disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains), sans restriction de pays ni de propriétaire. Les domaines inclus et leurs sous-domaines sont bloqués sans contacter de service externe.

Il s’agit d’un instantané publié avec la version, pas d’un répertoire exhaustif de chaque nouveau fournisseur. Les règles personnalisées peuvent le compléter. La compilation génère automatiquement le répertoire ignoré `internal/mail/data/` à partir des révisions et empreintes fixées dans `scripts/update-disposable-domains.mjs` ; les données locales vérifiées sont réutilisées hors ligne. Les licences figurent dans `THIRD_PARTY_NOTICES.md`.

## Fournisseurs et points d’accès

| Fournisseur | Préréglages et identifiants | Fonctions distantes |
| --- | --- | --- |
| SMTP | SMTP en clair, SSL/TLS implicite ou STARTTLS obligatoire ; mot de passe ou OAuth compatible | Estimations locales, réponse d’acceptation SMTP |
| Cloudflare Email | ID de compte, jeton API et point d’accès REST mondial | Résultat initial : distribué, rejeté ou en attente |
| Microsoft Graph | Outlook.com et Microsoft 365/Entra ; monde, gouvernement américain L4/L5 et Chine | État du dossier des messages envoyés |
| Amazon SES | Points d’accès régionaux IPv4, double pile et FIPS disponibles ; clé, secret et jeton temporaire facultatif | Quota d’envoi et suivi des messages |
| Twilio SendGrid | Points d’accès mondial et européen ; clé API | Crédits et statut Email Activity |
| Google Gmail | Gmail et Workspace ; OAuth délégué | Libellé de message envoyé |
| Alibaba Cloud Direct Mail | Hangzhou, Singapour, Virginie et Francfort ; accès publics et VPC ; clé et secret | Quota gratuit, solde et statistiques sans corrélation exacte |
| Tencent Cloud SES | API d’envoi et de facturation en Chine et à l’international ; clé et secret | Solde et statut de distribution par destinataire |
| Feishu / Lark Mail | Points d’accès Feishu et Lark ; OAuth utilisateur délégué | État de livraison au destinataire |

Consultez les catalogues des points d’accès [SES](https://docs.aws.amazon.com/general/latest/gr/ses.html) et [Direct Mail](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint).
Les accès VPC Direct Mail exigent une connexion dans la région correspondante. Les accès de Sydney retirés du service sont exclus.
Les préréglages SMTP couvrent Gmail, Outlook.com, Microsoft 365, QQ, NetEase 163/126, Cloudflare, SendGrid, SES, Direct Mail, Tencent et Feishu.
Les adresses et tarifs sont modifiables. Changer de préréglage charge ses valeurs ; changer de fournisseur ne conserve pas les identifiants de l’ancien fournisseur.

Les limites manuelles et de dépassement sont conservées. Un changement de devise du préréglage efface le solde manuel au lieu d’en réinterpréter les unités.

## Identifiants et permissions

Consultez [Permissions des API de messagerie](mail-api-permissions.md) pour les portées, actions IAM/CAM/RAM, jetons, prérequis et contrats vérifiés.
Les comptes Tencent API ordinaires exigent un modèle approuvé dans `tencent_template_id` ; le contenu personnalisé `Simple` est une ancienne fonction restreinte.

`smtp_security` accepte `plain`, `tls` ou `starttls`. Le port par défaut est 465 pour TLS implicite et 587 sinon.
TLS vérifie le certificat et le nom du serveur. STARTTLS est obligatoire lorsqu’il est sélectionné ; le mode en clair exige un choix explicite.
Utilisez un mot de passe d’application si nécessaire. OAuth SMTP accepte les jetons de renouvellement Gmail et Microsoft ; un jeton d’accès seul doit être renouvelé par l’administrateur.

Graph délégué utilise `client_id`, un `client_secret` facultatif et `refresh_token`. Les comptes personnels peuvent utiliser le locataire `common`.
L’accès applicatif exige un ID de locataire Entra et les identifiants du client ; `mailbox` doit alors désigner un utilisateur ou son adresse. L’accès délégué accepte `me`.
Accordez `Mail.Send` pour envoyer et la permission `Mail.Read` nécessaire aux recherches dans les messages envoyés. Les permissions applicatives exigent le consentement d’un administrateur.
Voir [Graph sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail) et les [déploiements nationaux](https://learn.microsoft.com/en-us/graph/deployments).

Gmail exige l’envoi délégué et l’accès aux métadonnées. Feishu/Lark exige un jeton utilisateur et les permissions d’envoi et de lecture.
RenoP renouvelle les jetons séquentiellement et enregistre les jetons de renouvellement remplacés avant tout envoi.
Les champs API sont `api_key`, `api_secret`, `session_token` facultatif, `account_id`, `region`, `endpoint` et `billing_endpoint` facultatif.
Le fournisseur doit autoriser l’adresse d’expédition. Les ID de compte, de boîte et les clés API ne sont pas interchangeables.

## Quotas et facturation

`quota.limit` définit l’allocation locale : négatif signifie illimité, zéro ou absent demande la découverte automatique, positif définit une allocation manuelle.
Sans quota distant disponible, le mode automatique démarre sans limite. Un recalage échoué conserve les limites connues, sans ajouter de crédits.
Les périodes manuelles sont les heures, jours, semaines commençant le lundi et mois calendaires en UTC. Les usages des quatre périodes sont conservés séparément.

`force_send` autorise le quota supplémentaire après épuisement du quota ordinaire.
`overage.limit` négatif permet un dépassement illimité ; zéro l’interdit. Sa période accepte `hour`, `day`, `week` ou `month`.
Les limites strictes du fournisseur s’appliquent toujours : payer un dépassement ne permet pas de contourner la limite technique SES.
Sans API de quota, renseignez manuellement l’allocation comprise dans votre abonnement.

Les limites cumulées `up_to` de `pricing.tiers` doivent croître. Seul le dernier palier peut être zéro, sans plafond.
Un palier facture `amount_micros` pour `batch_size` messages. Un lot de un correspond à une facturation par message.
`rounding: proportional` répartit le prix du lot ; `batch` facture un lot entier dès son premier message.
Prix et soldes sont exprimés en millionièmes de la devise `currency` à trois lettres : 1000000 vaut une unité. L’interface affiche les montants ordinaires.

Les préréglages sont datés du 2026-09-08 : [Cloudflare](https://developers.cloudflare.com/email-service/platform/pricing/) facture 0,35 USD par 1000 messages supplémentaires et [SES](https://aws.amazon.com/ses/pricing/) 0,10 USD par 1000 messages sortants.
Les [tarifs SendGrid](https://sendgrid.com/content/dam/sendgrid/global/en/other/sendgrid-pricing/twi121--sendgrid-pricing-pdf-st1.pdf) fournissent les valeurs modifiables Essentials 50K et Pro 100K ; l’envoi européen exige un abonnement admissible.
[Direct Mail](https://www.alibabacloud.com/help/en/direct-mail/billing-methods) utilise 0,29 USD par 1000 ; [Tencent Chine](https://cloud.tencent.com/document/product/1288/47930) 0,0019 CNY par message et [Tencent international](https://www-sg.tencentcloud.com/document/product/1084/39335) 0,00028 USD par message.
Les estimations marginales Graph, Gmail et Feishu/Lark valent zéro ; leurs limites d’abonnement restent applicables.
Ces estimations excluent abonnements, taxes, données des pièces jointes et options. Adaptez le modèle à votre contrat.

Un `balance_micros` vide indique un solde inconnu. Un solde connu nul, négatif ou insuffisant pour le prochain supplément suspend l’envoi.
`fetch_balance` permet le recalage d’un solde fournisseur compatible dans la devise configurée. Une donnée indisponible ne devient pas un solde nul.
Quota et coût supplémentaire sont réservés avant l’envoi. Identifiants invalides, configuration d’expéditeur invalide et connexion échouée avant soumission ne consomment pas le quota.
Les autres tentatives, y compris rejet du destinataire et résultat indéterminé, sont décomptées. Un échec explicitement non facturable est remboursé une seule fois ; la limite de tentatives reste débitée.
Une vérification tardive n’ajoute pas de crédits distants si un recalage plus récent inclut déjà cet ajustement.

`calibration` vaut cinq minutes par défaut et accepte minutes, heures ou jours. Vide, zéro ou négatif désactive les requêtes de quota et de solde.
RenoP décompte localement entre les recalages. Lorsqu’il est activé, le premier relevé précède le premier envoi du compte.
Les fournisseurs sans API publique de quota ou de solde utilisent les estimations locales et la configuration manuelle.

## File et statut de distribution

Envoi, renouvellement OAuth, recalage et recherche de statut partagent un seul traitement séquentiel. Le délai par défaut est de cinq secondes après chaque tentative terminée.
La file et les compteurs survivent au redémarrage. Un bail en base empêche plusieurs travailleurs de soumettre simultanément.
Une soumission interrompue avant l’enregistrement de son résultat devient `unknown` et n’est jamais renvoyée automatiquement.

`accepted` indique l’acceptation du fournisseur ; `sent` indique une vérification réussie du dossier ou de l’état envoyé.
`delivered` exige un résultat de distribution explicite. Attente, pause, vérification, échec, expiration, annulation et état inconnu restent distincts.
SES et SendGrid exigent les fonctions et permissions de suivi correspondantes. L’état envoyé de Graph et Gmail ne prouve pas la réception.
Feishu/Lark utilise l’API `send_status` par destinataire pour distinguer livraison, rejet et traitement en attente.
Les statistiques publiques Direct Mail omettent les ID de message : RenoP les consulte puis retourne `unknown`, sans attribuer le résultat d’un autre message.
Cloudflare fournit directement son résultat initial ; aucun point d’accès de suivi non pris en charge n’est utilisé.

Les recherches suivent la soumission avec un délai croissant borné. Des échecs répétés ou l’expiration produisent un résultat inconnu et un journal global.
Les codes d’échec sont conservés dans les journaux administrateur. Le statut public omet destinataires, contenu, identifiants et diagnostics bruts.

## Modèles et notifications

`template_style` accepte `card`, `compact` et `notice`. Les modèles reprennent les surfaces neutres, cartes arrondies, boutons en capsule et palettes claire/sombre du service. Les e-mails suivent la langue enregistrée du compte destinataire ; les nouvelles inscriptions et les destinataires sans préférence utilisent l’en-tête `Accept-Language` de la page d’origine, avec `en-US` par défaut. Les 12 langues de l’interface sont disponibles. La langue est fixée à la mise en file. L’ancien réglage global `locale` est ignoré et n’apparaît plus dans les paramètres.
Chaque style comporte du texte brut, des substitutions échappées et des liens HTTPS limités à l’instance configurée.

```text
registration_verify, registration_success, password_reset, password_changed,
email_verify, email_changed, quota_changed, review_status, review_requested,
permission_changed, account_banned, account_unbanned, collaboration_invitation,
super_team_invitation, pending_reviews, unusual_login, security_changed,
account_retired, notification, test
```

Les changements de mot de passe et Passkey, permissions, bannissements, quotas utilisateur, révisions, invitations et messages internes utilisent des événements persistants.
Les notifications sont dédupliquées ; la première activation ne rejoue pas l’historique des actions.
Le signal de nouveau réseau compare le réseau IPv4 /24 ou IPv6 /56 à la connexion précédente ; il ne représente pas une localisation géographique.
Avant l’envoi, les messages liés à un compte revérifient son existence et son adresse de sécurité. La clôture supprime ses messages en attente.

## API d’administration

```http
GET /api/settings/mail
PUT /api/settings/mail
GET /api/settings/mail/presets
POST /api/settings/mail/test
GET /api/settings/mail/accounts/:id
GET /api/settings/mail/jobs?limit=20&offset=0&status=failed
GET /api/settings/mail/templates/:scene?style=card
GET /api/auth/mail/:id
```

Les interfaces de paramètres exigent les droits administrateur. Le JSON remplace toute la configuration de courriel ; limite du corps : 1 MiB.
GET et PUT réussi omettent les valeurs secrètes et renvoient `secrets_configured`, associant chaque ID de compte aux champs configurés.
Pour un fournisseur inchangé, une chaîne secrète vide conserve sa valeur. Utilisez `clear_secrets: {"primary":["password"]}` pour l’effacer explicitement.
Les sept champs accessibles uniquement en écriture sont `password`, `api_key`, `api_secret`, `session_token`, `client_secret`, `access_token` et `refresh_token`.
Cette API ne lit ni ne remplace la clé de chiffrement persistante.

Le test utilise les paramètres enregistrés et accepte ce JSON :

```json
{"account_id":"primary","to":"receiver@example.com"}
```

Une mise en file réussie retourne HTTP 202, sans confirmer la distribution :

```json
{"id":"opaque-job-id","status":"queued","ticket":"private-status-capability"}
```

Interrogez l’interface de statut avec l’en-tête `X-Renop-Mail-Ticket` reçu, ou avec la session du propriétaire.
Gardez ce jeton privé et hors des URL. Un propriétaire ou jeton invalide produit 404 ; un stockage indisponible produit 503.
L’état du compte contient usages par période, tentatives, quotas restants estimés, solde, coût supplémentaire et dernier recalage.
Les tâches acceptent `limit` de 1 à 50, `offset` de 0 à 10000 et un filtre exact `status`. Les aperçus fournissent sujet, HTML et texte.

Pour SMTP, les identifiants enregistrés sont conservés uniquement si l’hôte et le nom d’utilisateur restent inchangés.

## Stockage et limites

Maximum : 64 comptes, 1000 entrées de destinataires et 20 paliers tarifaires par compte.
Les périodes sont limitées à un an et les volumes de quota ou de palier à un milliard de messages.
Texte et HTML réunis, ainsi que chaque réponse fournisseur, sont limités à 128 KiB. Une tâche a un destinataire et expire sous 24 heures au maximum.
La file accepte 2048 tâches actives et 12048 enregistrements au total. La maintenance réduit l’historique terminé vers 8000 lignes et supprime les entrées de plus de sept jours.
Les compteurs IP expirent ; l’état d’un compte supprimé est nettoyé après 24 heures.

Les contenus et identifiants renouvelés sont chiffrés avec `mail.encryption_key`, conservé dans le fichier de configuration privé.
Sauvegardez cette clé avec la base. Sa perte ou son remplacement empêche le déchiffrement des tâches et états de compte.
Les tâches finalisées par le travailleur perdent leur HTML et texte ; les soumissions interrompues gardent leur contenu chiffré jusqu’au nettoyage.
Les journaux et API de statut n’exposent ni corps de message ni adresse destinataire.
