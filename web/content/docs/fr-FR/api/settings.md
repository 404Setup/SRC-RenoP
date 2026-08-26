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

## Lire et modifier un domaine

- **Lire** : `GET /api/settings/domain/:name`
- **Modifier** : `PUT /api/settings/domain/:name`
- **Comportement** : le schéma dépend de `:name`. Les champs inconnus et valeurs invalides sont refusés. Les changements
  d’hôte, port, TLS, base de données ou certains paramètres d’exécution peuvent imposer un redémarrage.
- **GitHub OAuth** : `GET /api/settings/github-oauth` renvoie un état masqué et `PUT /api/settings/github-oauth` modifie
  l’identifiant client et le secret en écriture seule.

## Paramètres des dépôts

Préférez `/api/settings/repositories`. Les alias préfixés par Maven restent disponibles pour compatibilité.

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
