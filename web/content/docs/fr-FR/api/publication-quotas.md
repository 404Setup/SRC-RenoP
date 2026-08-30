---
title: Quotas de publication
order: 18
category: Référence API
description: Limites périodiques pour les comptes et les équipes globales
---

# Quotas de publication

Les quotas limitent le nombre de fichiers locaux, leur taille cumulée et les publications complètes. Une nouvelle
installation autorise par défaut 600 fichiers, 40 Mio et 20 publications par mois. Un administrateur système peut
modifier ces valeurs ou définir une règle propre à un compte ou à une équipe globale.

## Politique

`period` accepte `day`, `week` ou `month` et utilise des limites UTC. Une valeur nulle n'est admise que dans une règle
propre à un propriétaire et interdit alors l'opération. L'option `unlimited`, réservée aux administrateurs, désactive la
consommation. Un objet vide restaure toutes les valeurs globales.

## Propriété

Un paquet personnel consomme le quota du compte qui publie. Un paquet ou domaine Maven lié à une équipe globale consomme
uniquement le quota de cette équipe. Un transfert ne modifie que les publications futures et ne déplace pas l'historique.
Les téléchargements et catalogues provenant d'un miroir ne consomment aucun quota.

## Comptabilisation

Cargo et npm comptent un fichier stocké et une publication par version acceptée. Docker compte le manifeste, la
configuration et les descripteurs de couches, puis termine une publication lors de l'envoi du manifeste. Maven compte
chaque PUT client comme fichier et une publication lorsque le POM est accepté. Le moteur de fichiers compte chaque PUT
comme un fichier et une publication. Les index et sommes de contrôle générés par le serveur ne sont pas ajoutés.

Les envois concurrents créent d'abord une réservation persistante et temporaire. Une validation réussie la confirme ;
une réservation abandonnée est libérée ou supprimée par la maintenance. L'état inclut l'utilisation confirmée et les
réservations actives afin que des requêtes parallèles ne dépassent pas la limite.

## Points de terminaison

```http
GET /api/publication-quota
GET /api/publication-quota/users/{username}
PUT /api/publication-quota/users/{username}
GET /api/publication-quota/super-teams/{prefix}
PUT /api/publication-quota/super-teams/{prefix}
GET /api/settings/publication-quota
PUT /api/settings/publication-quota
```

Un compte lit son propre état et les membres lisent celui de leur équipe. Seul un administrateur système peut consulter
un autre compte, modifier les règles, activer `unlimited` ou changer les valeurs globales.

## Application

Un quota épuisé renvoie `429 Too Many Requests`. `X-Renop-Error-Code` distingue `publication_file_quota`,
`publication_byte_quota` et `publication_count_quota`. Les quotas s'appliquent après l'authentification, les permissions,
la réservation du paquet, la liaison d'espace de noms et la vérification Maven ; ils n'accordent aucun droit.
