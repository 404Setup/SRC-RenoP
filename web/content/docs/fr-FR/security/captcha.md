---
title: Vérification de sécurité
order: 16
category: Security
description: Configurer les fournisseurs CAPTCHA et les actions protégées
---

# Vérification de sécurité

## Fournisseurs et actions

Configurez CAPTCHA dans les paramètres de vérification. Les modes sont désactivé, reCAPTCHA v2 avec case, reCAPTCHA v2 invisible, reCAPTCHA v3, Cloudflare Turnstile, hCaptcha et Friendly Captcha v2. Utilisez les clés du site et secrètes/API correspondantes.

Activez séparément connexion par mot de passe, inscription, courriel manuel, création d’équipe globale, de domaine et de paquet. Passkey et connexion externe ne sont pas concernés par le contrôle du mot de passe. Pour le courriel d’inscription, le contrôle manuel est prioritaire, sinon celui d’inscription. Terminer l’inscription est une action distincte.

Les contrôles concernent les actions interactives ou anonymes. Les jetons API et mots de passe de protocole validés restent exemptés selon les identifiants vérifiés, jamais le User-Agent. Les envois Maven du navigateur et leur initialisation par blocs vérifient les nouveaux paquets. Les paquets existants et publications automatisées gardent leurs permissions.

Les secrets ne sont jamais renvoyés. Un champ vide conserve le secret si le fournisseur et la clé du site restent identiques ; changer l’un d’eux, y compris désactiver, efface l’ancien secret. Le seuil v3 vaut 0.5 par défaut. Autorisez les noms d’hôte dans les domaines du serveur et chez le fournisseur. Friendly Captcha propose les régions globale et UE.

reCAPTCHA et Turnstile vérifient le nom d’hôte. hCaptcha vérifie la clé de site attendue, son champ de nom d’hôte étant statistique. Friendly Captcha vérifie la clé de site et l’origine lorsque le fournisseur la renvoie.

```yaml
captcha:
  provider: turnstile
  site_key: YOUR_SITE_KEY
  secret_key: YOUR_SECRET_KEY
  min_score: 0.5
  friendly_region: global
  scopes:
    password_login: true
    registration: true
    manual_mail: true
    super_team_create: false
    domain_create: false
    package_create: false
```

## Vérification du navigateur

Le code tiers ne charge dans un cadre isolé qu’après accord dans les préférences des cookies. Annulation, navigation et retrait du consentement suppriment ce cadre. Une validation réussie du fournisseur est obligatoire ; erreurs, scores faibles, mauvaises actions et mauvais hôtes sont refusés.

Le navigateur ne réessaie qu’une fois après une demande explicite de CAPTCHA, avant tout début de mutation. Les erreurs réseau, résultats inconnus d’envoi de courriel et autres échecs ne déclenchent aucune répétition automatique.

## Contrat API

`GET /api/captcha` renvoie la configuration publique et pose le cookie HttpOnly nécessaire. `POST /api/captcha/verify` accepte les champs JSON `scope`, `provider`, `site_key` et `response`, avec une réponse CAPTCHA limitée à 16 KiB et un corps JSON à 32 KiB. Le succès renvoie un `proof` à usage unique valable 120 secondes.

Envoyez ce justificatif dans `X-Renop-Captcha` avec les mêmes cookies. Il est lié à l’action, à la session et à la configuration. Une preuve absente donne `428`, `X-Renop-Error-Code: captcha_required` et `X-Renop-Captcha-Scope` ; une preuve invalide ou réutilisée donne `400`. Les pannes du fournisseur donnent `503`.

`GET /api/settings/captcha` et `PUT /api/settings/captcha` exigent les droits de gestion des paramètres et utilisent JSON. Aucun secret n’est renvoyé. Les changements sont immédiats et invalident les justificatifs en attente.

[reCAPTCHA](https://developers.google.com/recaptcha/docs/verify) · [reCAPTCHA v3](https://developers.google.com/recaptcha/docs/v3) · [Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/) · [hCaptcha](https://docs.hcaptcha.com/) · [Friendly Captcha](https://developer.friendlycaptcha.com/docs/v2/getting-started/verify)
