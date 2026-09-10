---
title: Documents juridiques et cookies
order: 15
category: Configuration
description: Configurer les politiques, leur acceptation et les préférences du navigateur
---

# Documents juridiques et cookies

## Pages des politiques

Les administrateurs modifient la politique de confidentialité, les conditions de service et les mentions légales dans les paramètres des documents juridiques. Les trois utilisent le même éditeur Markdown et un aperçu sécurisé, avec une limite de 512 KiB UTF-8 par document. Un champ vide restaure le texte provisoire à remplacer.

Les documents sont stockés dans `legal` de `config.yaml` et prennent effet après enregistrement. Le fichier de confidentialité historique n’est plus lu : copiez son contenu dans les paramètres avant la mise à niveau. L’ancienne URL externe des mentions légales est retirée. Les fichiers existants sont conservés.

Les pages publiques sont `/privacy-policy`, `/terms-of-service` et `/legal-notice`, accessibles même avec des identifiants expirés. `GET /api/legal` renvoie la révision actuelle et `cookie_banner` ; `GET /api/legal/:document` renvoie du texte brut borné. `GET /api/privacy-policy` reste un alias.

`GET /api/settings/legal` et `PUT /api/settings/legal` exigent le droit d’administrer les paramètres et utilisent les champs JSON `privacy_policy`, `terms_of_service`, `legal_notice` et `cookie_banner`.

## Connexion et inscription

La connexion et l’inscription exigent une acceptation explicite des politiques actuelles, y compris les mots de passe, Passkeys, fournisseurs et seconds facteurs. L’authentification des clients de paquets conserve les règles de protocole existantes.

Les clients obtiennent la révision avec `GET /api/legal` et l’acceptent via `X-Renop-Legal-Revision` ou le cookie de consentement. Une acceptation absente ou périmée renvoie HTTP 428 avec `X-Renop-Error-Code: legal_consent_required`. L’événement d’audit existant consigne la révision acceptée lors du succès.

## Choix des cookies

Le bandeau propose les cookies nécessaires uniquement, tout accepter et les préférences par catégorie. Les cookies nécessaires assurent les sessions et la sécurité ; les vérifications tierces optionnelles exigent un choix explicite. Les préférences restent accessibles dans le pied de page, même si le bandeau est désactivé.

Les choix restent dans le navigateur un an et expirent lors d’une modification des politiques. Refuser les services optionnels est un choix valide ; les intégrations ne doivent pas les charger avant l’accord.
