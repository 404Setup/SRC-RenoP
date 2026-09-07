---
title: Moteurs de cache
order: 5
category: Configuration
description: Caches mémoire, Redis et Valkey avec invalidation des identifiants
---

# Moteurs de cache

RenoP utilise la mémoire par défaut. Les administrateurs peuvent sélectionner Redis ou Valkey dans les paramètres du service.
Le changement de moteur prend effet après un redémarrage.

## Configuration

```yaml
cache:
  mode: memory
  address: localhost:6379
  username: ""
  password: ""
  database: 0
  tls: false
  timeout_ms: 1000
```

`mode` accepte `memory`, `redis` ou `valkey`. `address` utilise `host:port`, avec des crochets pour IPv6.
Le nom d’utilisateur est facultatif ; l’authentification par mot de passe et TLS sont pris en charge. TLS vérifie le certificat.
Le numéro de base doit être compris entre 0 et 65535 et exister sur le service choisi.
`timeout_ms` accepte 10–10000 millisecondes pour la connexion, l’attente du pool, la lecture et l’écriture.
Le pool utilise au plus huit connexions. Un moteur externe doit être joignable au démarrage.

## Données en cache

Le moteur s’applique au contenu des métadonnées d’artefacts, aux métadonnées Maven analysées, aux résultats
d’authentification, aux recherches de comptes, sessions, profils et identités, aux politiques de cache des dépôts,
aux jetons Docker en amont et aux ressources SPA. Le document HTML généré utilise aussi ce moteur.
Le stockage durable, les sessions actives, verrous, connexions et tâches conservent leur gestion actuelle.

Les valeurs externes sont chiffrées et authentifiées avec une clé propre au processus. Les clés de cache sont opaques
et ne contiennent ni identifiants ni chemins de requête. Les index locaux conservent uniquement les champs nécessaires
à l’éviction bornée et à l’invalidation ciblée. La révocation supprime les références même si la suppression distante échoue.
Une authentification commencée avant l’invalidation ne peut pas remplir de nouveau le cache.

Les capacités et règles d’expiration existantes restent appliquées. La capacité des fichiers suit
`server.file_cache_size_mb`. Une valeur externe est limitée à 2 Mio et expire au plus tard après 24 heures ;
les fichiers, politiques et ressources SPA expirent après une heure. Configurez la limite mémoire et la politique
d’éviction du serveur de cache. RenoP ne modifie pas les paramètres d’un serveur partagé.

Un redémarrage du cache, une éviction, une entrée manquante, un chiffrement invalide ou une erreur de connexion à l’exécution
déclenche le recours à la base, au stockage ou au générateur. Après une erreur réseau, le cache est contourné cinq secondes.
Un redémarrage de RenoP crée un nouvel espace de noms et une nouvelle clé ; les anciennes valeurs expirent naturellement.
Aucune valeur de cache ne fait autorité.

## API d’administration

```http
GET /api/settings/cache
PUT /api/settings/cache
POST /api/settings/cache/test
```

Ces endpoints exigent un administrateur système ou un jeton autorisé avec `admin:settings`.
GET et PUT ne renvoient jamais le mot de passe enregistré. `password_configured` indique sa présence ;
`restart_required: true` signifie qu’un redémarrage est nécessaire. Un mot de passe vide conserve la valeur existante ;
`clear_password: true` la supprime.

PUT accepte les champs ci-dessus. Le test utilise les mêmes paramètres, conserve le mot de passe omis et vérifie
la connexion sans enregistrer. Le corps est limité à 8 Kio.
Les erreurs stables sont `cache_settings_invalid`, `cache_settings_save_failed` et `cache_connection_failed`.

Le compte de cache doit pouvoir exécuter PING, SET, GETRANGE et DEL sur `renop:*`, ainsi que les commandes
d’authentification et de sélection de base nécessaires. Ces endpoints ne configurent pas Sentinel ni la découverte de cluster.
