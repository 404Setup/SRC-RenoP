---
title: API des statistiques de téléchargement
order: 14
category: Référence API
description: Comptage borné, requêtes hiérarchiques, contrôles par dépôt et exigences des jetons API
---

# API des statistiques de téléchargement

RenoP agrège les téléchargements de paquets réussis sans créer une ligne de base de données par requête. Les compteurs
contiennent le nombre de téléchargements, les octets logiques et la dernière date de mise à jour. L’attribution utilise
l’identité immuable du compte ; un changement de nom ne divise donc pas l’historique.

Les dépôts Maven, npm, Cargo et Docker comptent par défaut. Le moteur non structuré `files` doit être activé. Les requêtes de
sommes de contrôle, signatures détachées, métadonnées Maven et compagnons Javadoc sont exclues. Les requêtes `HEAD`, les
réponses `304`, les échecs et les segments de plage non initiaux ne sont pas comptés. Docker enregistre un pull lors du
retour du manifeste, sans compter chaque blob.

## Requêtes du compte

`GET /api/statistics` renvoie les statistiques du propriétaire du jeton API. `GET /api/statistics/users/:username`
utilise la même limite de compte ; consulter un autre compte exige un jeton d’administrateur système.

Les deux routes exigent un jeton Bearer avec `statistics:read`. Les cookies de session navigateur et les identifiants
Basic sont refusés. Les compteurs en mémoire sont écrits avant la requête ; la réponse inclut donc les téléchargements
déjà acceptés par le processus serveur courant.

## Requêtes système

`GET /api/statistics/system` exige un compte administrateur système et la portée `admin:statistics`. Le regroupement
peut être `user`, `repository`, `namespace`, `package` ou `version`. Les routes de compte acceptent tous ces niveaux
sauf `user`.

Les filtres exacts facultatifs sont `username` (système uniquement), `repository`, `format`, `namespace`, `package` et
`version`. La pagination utilise un `limit` de 1 à 100 et un `offset` compris entre 0 et 1 000 000. Chaque page renvoie
aussi les totaux `count` et `bytes` du filtre complet, ainsi que le nombre total de groupes.

## Contrôles du dépôt

Les administrateurs lisent les commutateurs effectifs avec `GET /api/settings/repositories/download-statistics` et
modifient un dépôt avec `PUT /api/settings/repositories/:name/download-statistics`. Le corps JSON est
`{"enabled": true}` ou `{"enabled": false}`.

`DELETE /api/settings/repositories/:name/download-statistics` efface définitivement les compteurs enregistrés et en
attente. Pour Docker, il remet aussi à zéro le compteur de compatibilité affiché sur les images. La suppression d’un
dépôt efface automatiquement ses statistiques.

Les schémas de réponse et limites complets figurent dans `web/assets/openapi.yaml`.
