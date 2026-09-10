---
title: API des paramètres
order: 8
category: Référence API
description: Paramètres de service par domaine, dépôts et reconstruction d’index
---

# API des paramètres

Les routes exigent un compte administrateur ou un API Token avec `admin:settings` ou `admin:repositories`, selon
l’opération. Les réponses utilisent protobuf lorsque `proto/api/v1/api.proto` le prévoit.

## Découvrir les domaines de paramètres

- **Chemin** : `GET /api/settings/domains`
- **Réponse** : noms stables pris en charge, notamment `server`, `proxy`, `storage`, `updater` et `index`.

## Pages de paramètres dans le navigateur

Chaque domaine parmi les 14 annoncés dispose de sa propre page. Sur ordinateur, les catégories figurent à côté
du formulaire ; sur petit écran, un sélecteur les remplace. Précédent et suivant suivent le même ordre. Ouvrir une
page charge uniquement sa configuration, sans charger tous les paramètres du service.

Chaque page conserve son brouillon lors du changement de catégorie ou de langue. L’enregistrement met à jour
uniquement la page active et bloque temporairement la saisie et la navigation. Un échec conserve le brouillon ;
l’abandon recharge cette page après confirmation. La navigation signale les changements non enregistrés. Le
rechargement ou la fermeture du navigateur avertit des modifications, mais les brouillons restent seulement en
mémoire et sont effacés à la déconnexion ou au changement de compte. Les secrets déjà stockés restent masqués et
les secrets saisis sont retirés du brouillon après un enregistrement réussi.

GPG reste dans la configuration du service. Limites d’équipe, quotas, inscription, cache, courriel, fournisseurs
OAuth et sécurité des domaines ont leurs propres pages et conservent leurs API JSON. Les libellés et indications
sont associés aux commandes ; la navigation place le focus clavier sur le titre de la nouvelle page.

La catégorie active, les marqueurs de brouillon et le texte secondaire utilisent les couleurs communes des thèmes clair
et sombre.

## Lire et modifier un domaine

- **Lire** : `GET /api/settings/domain/:name`
- **Modifier** : `PUT /api/settings/domain/:name`
- **Comportement** : le schéma dépend de `:name`. Les champs inconnus et valeurs invalides sont refusés. Les changements
  d’hôte, port, TLS, base de données ou certains paramètres d’exécution peuvent imposer un redémarrage.
- **GitHub OAuth** : `GET /api/settings/github-oauth` renvoie un état masqué et `PUT /api/settings/github-oauth` modifie
  l’identifiant client et le secret en écriture seule.

**Autres fournisseurs OAuth** : `GET /api/settings/oauth-providers` renvoie les clients et préréglages sans secrets ;
`PUT /api/settings/oauth-providers` remplace la liste. Le tableau `providers` est obligatoire ; un tableau explicitement
vide supprime tous les clients configurés. Jusqu’à 32 clients et un corps de 128 KiB sont acceptés.
Voir [Connexion via un service tiers](../security/oauth-login.md) pour les identifiants, préréglages et associations.

## Paramètres des dépôts

Préférez `/api/settings/repositories`. Les alias préfixés par Maven restent disponibles pour compatibilité.

Les changements sont validés en base avant de remplacer la configuration active. La suppression du dernier dépôt
reste effective après redémarrage ; le YAML hérité sert uniquement à la migration initiale.

### Lister les dépôts

- **Chemin** : `GET /api/settings/repositories`
- **Alias** : `GET /api/settings/maven/repositories`

### Créer, modifier, supprimer ou migrer

- **Créer ou modifier** : `PUT /api/settings/repositories/:name`
- **Supprimer** : `DELETE /api/settings/repositories/:name`
- **Migrer Maven/files** : `POST /api/settings/repositories/:name/migrate/:target`, avec `maven` ou `files`. Les objets
  ne sont pas déplacés ; le catalogue Maven est reconstruit lors du retour vers Maven.

## Reconstruire l’index de recherche

- **Chemin** : `POST /api/settings/index/rebuild`
- **Comportement** : soumet une reconstruction fusionnée en arrière-plan, sans lancer deux tâches concurrentes.

## Réservation des domaines de publication

`GET /api/settings/maven-domains` et `PUT /api/settings/maven-domains` utilisent JSON.
La découverte contient `maven_domains`. Valeur par défaut :

```json
{"release_value":2,"release_unit":"year"}
```

`release_value` est un entier de 1 à 100 ; `release_unit` accepte `month` ou `year`, selon le calendrier UTC.
Le fichier de configuration conserve ces champs sous `maven_domains`.
Une modification enregistrée concerne les nouveaux verrouillages de sécurité, sans modifier les dates existantes
ni le délai distinct de 31 jours d’une fermeture volontaire. Voir l’[état des domaines Maven](maven.md).
