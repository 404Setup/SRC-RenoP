---
title: Équipes globales
order: 12
category: Référence API
description: Préfixes immuables, rôles T1-T4, invitations et limites de compte
---

# Équipes globales

Une équipe globale est une identité de collaboration valable pour toute l’instance. Son préfixe immuable peut être
référencé par les moteurs de paquets sans recopier ses membres dans chaque paquet. Les identités internes restent
immuables ; les réponses exposent uniquement les noms d’utilisateur.

## Rôles et propriété

T1 lit selon la visibilité du paquet. T2 publie et maintient les versions. T3 gère les membres T1/T2 et crée des
paquets pour l’équipe. T4 possède la configuration et peut accorder T3/T4.

Il doit toujours rester un propriétaire T4. T3 ne peut ni modifier ni accorder T3/T4. Un administrateur système gère
toutes les équipes sans les rejoindre, mais l’ajout respecte toujours la limite du compte cible. S’ajouter soi-même ne
produit pas de message redondant.

## Liaisons de projets et de domaines

GET /api/super-teams/eligible renvoie par défaut les équipes où l’appelant est au moins T3 ; `minimum_role` accepte
T1-T4 pour sélectionner une cible de transfert. Une image Docker avec une barre oblique doit sélectionner l’équipe du
premier segment. Un paquet npm avec portée doit sélectionner l’équipe de la portée sans `@`. Les noms sans préfixe
peuvent rester personnels.

La même liaison s’applique aux crates Cargo, aux artefacts Maven et aux domaines de publication Maven. L’accès effectif
est le niveau le plus élevé entre l’autorisation explicite et le rôle d’équipe mappé. Les membres ne sont jamais copiés
dans les tables de membres des paquets. Une équipe liée ne peut être supprimée avant le transfert de toutes ses ressources.
Un propriétaire L4 demande le transfert dans `/account/reviews` ; un gestionnaire T3/T4 ou administrateur système le décide une seule fois.

## Limites

`super_teams.create_limit` et `super_teams.join_limit` valent respectivement cinq et vingt par défaut. Une équipe
possédée compte dans les deux usages.

GET /api/super-teams/limits retourne les limites effectives. Les gestionnaires utilisent GET
/api/super-teams/users/{username}/limits et PUT /api/super-teams/users/{username}/limits pour une dérogation. `-1`
restaure l’héritage ; zéro interdit l’action. GET /api/settings/super-teams et PUT /api/settings/super-teams configurent
les valeurs globales.

## Cycle de vie

GET /api/super-teams retourne une page triée par préfixe. Un utilisateur ne voit que ses équipes ; un administrateur
les voit toutes. POST /api/super-teams réserve un préfixe et crée le demandeur comme T4. Le préfixe comporte 2 à 64
lettres minuscules, chiffres, tirets ou traits de soulignement, commence et finit par une lettre ou un chiffre, et ne
peut plus changer.

GET /api/super-teams/{prefix} retourne les métadonnées et les noms des membres. PUT /api/super-teams/{prefix} modifie
le nom et la description. DELETE /api/super-teams/{prefix} supprime l’équipe et annule atomiquement ses invitations.

## Adhésions

POST /api/super-teams/{prefix}/members accepte de un à vingt utilisateurs et un rôle T1-T4. Les gestionnaires créent
une invitation du centre de messages, valable sept jours et utilisable une fois ; l’administrateur ajoute directement.

PUT /api/super-teams/{prefix}/members/{username} modifie le rôle. DELETE
/api/super-teams/{prefix}/members/{username} retire le membre ou quitte l’équipe. POST
/api/super-teams/invitations/{id}/{decision} accepte `accept` ou `reject` et garantit qu’une réponse concurrente ne
s’applique pas deux fois.

## Frontière des API Tokens

Les routes exigent `team:manage` et ciblent précisément `global/{prefix}`. La lecture des limites exige `account:read`,
la dérogation `admin:users`, et les réglages globaux `admin:settings`. Un Token ciblé ne peut ni lister toutes les
équipes ni créer un autre préfixe.

Les erreurs exposent un `X-Renop-Error-Code` stable et un corps générique borné. Les clients utilisent le statut HTTP
et le code enregistré, jamais le texte brut.
