---
title: API du registre Maven
order: 4
category: Référence API
description: Domaines vérifiés, équipes, catalogues d’artefacts et accès des clients Maven
---

# API du registre Maven

Les dépôts Maven RenoP utilisent des espaces de noms en domaine inversé vérifiés. Un éditeur réserve son domaine une
seule fois depuis le menu du compte, puis peut publier dans tous les dépôts Maven autorisés. Les chemins Maven 2,
métadonnées, signatures détachées et sommes de contrôle restent compatibles avec Maven et Gradle.

## Vérification d’un domaine

Créez le domaine avec `POST /api/maven/domains`. RenoP renvoie un code aléatoire robuste et une cible de preuve :

- les espaces DNS placent un TXT à la racine enregistrée ; toutes les valeurs TXT sont lues et la correspondance doit
  être exacte ;
- `io.github.<account>` utilise la Bio d’un utilisateur GitHub public ou la Description d’une organisation publique ;
- `io.gitlab.<account>` utilise la Bio d’un utilisateur GitLab public ou la Description d’un groupe public.

Lancez la vérification avec `POST /api/maven/domains/:domain/verify`. Une tentative est admise toutes les cinq secondes
par domaine. Un administrateur peut utiliser `/verify/force`; l’action est inscrite dans le journal d’audit.

Le domaine vérifié et son équipe sont globaux à l’instance. Ils sont réutilisés dans tous les dépôts Maven sans nouvelle
preuve ni nouvelle invitation.

Une association récente à GitLab.com peut aussi vérifier automatiquement un nouvel espace : le compte doit posséder l’espace personnel ou un groupe de premier niveau, avec une preuve datant de moins d’une heure. L’appartenance publique, la propriété d’un sous-groupe et les instances auto-hébergées ne suffisent pas. Voir [Connexion via un service tiers](../security/oauth-login.md).

## Autorisations du domaine

Les équipes Maven appartiennent au domaine global :

- L0 : lecture du contenu public ;
- L1 : publication d’artefacts ;
- L2 : gestion des versions et descriptions ;
- L3 : invitation et gestion des membres ;
- L4 : propriété et transfert du domaine.

Une requête d’invitation accepte de un à vingt noms. Hors administration, l’ajout passe par le centre de messages. Un
transfert conserve exactement un propriétaire L4, et le propriétaire doit transférer le domaine avant de le quitter.

## Catalogue d’artefacts

`GET /api/maven/repositories/:repo/domains` liste les domaines qui ont des artefacts dans le dépôt.
`GET /api/maven/repositories/:repo/packages` fournit la recherche paginée.
`GET /api/maven/repositories/:repo/package?group=...&artifact=...` renvoie l’artefact et ses versions. Les membres L2
peuvent modifier la description et supprimer une version complète via les routes JSON associées.

La réponse détaillée résume les fichiers principaux indexés, leurs tailles, dates de modification, sommes de contrôle
et signatures détachées. Elle renvoie au plus 64 fichiers principaux par version. Si le dernier POM indexé ne dépasse
pas 2 Mio, RenoP le lit en flux et expose aussi le projet, l’organisation, les licences, les développeurs, le contrôle
de source, le suivi des problèmes, le parent et les dépendances directes. Les dépendances directes sont limitées à 128
entrées ; les fichiers compagnons de somme de contrôle et de signature ne sont pas comptés comme fichiers principaux.

Un membre L2-L4 du domaine ou un administrateur peut gérer un README Markdown distinct au niveau du paquet via
l’endpoint de mise à jour de l’artefact. Il est limité à 512 Kio, renvoyé uniquement par l’endpoint détaillé et rendu
avec la liste commune d’éléments et d’URL autorisés. La description courte du POM ou du catalogue reste indépendante.

Les anciens dépôts sont indexés lors de la mise à niveau. Les domaines importés sont vérifiés mais n’obtiennent aucun
membre automatiquement. Les miroirs Maven configurés continuent de résoudre les artefacts absents.

## Présentations et dépôts de fichiers

L’interface moderne affiche le catalogue par domaine. Un administrateur peut choisir l’arborescence classique et
revenir ensuite au catalogue. Cette option ne change que la présentation : les chemins arbitraires restent refusés et
la publication exige toujours un domaine vérifié et un chemin Maven valide.

Le format distinct `files` sert au contenu non structuré. Il autorise remplacement, suppression, S3 et miroirs, sans
génération de sommes, génération de POM ni validation OpenPGP.

## Accès Maven et Gradle

Les lectures et publications utilisent `/{repo}/{maven-path}`. Utilisez un mot de passe ou un API Token avec
`repository:read` et/ou `repository:publish`. La visibilité contrôle la lecture ; le domaine vérifié et le niveau L0-L4
du compte contrôlent les mutations. Le contrat complet se trouve dans `web/assets/openapi.yaml`.

## Verrouillage des ressources

Les administrateurs et les modérateurs du dépôt gèrent les verrous des artefacts ou versions avec un cookie de session navigateur actif :

- `PUT /api/maven/repositories/{repo_name}/package/locks?group={group}&artifact={artifact}`
- `DELETE /api/maven/repositories/{repo_name}/package/locks?group={group}&artifact={artifact}`

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

Omettre `version` ou utiliser une chaîne vide cible l’artefact. DELETE accepte `{"version":"1.2.3"}` et retire uniquement ce verrou manuel ; les verrous système restent indépendants. Les API Tokens ne peuvent pas gérer les verrous. L’interface traduit les motifs publics : `hold`, `prohibited`, `expired`, `trojan`, `abuse`, `dmca`, `reup`, `squatting` et `quality`.

Les deux modes bloquent les écritures. `write` conserve les téléchargements stockés ; `read` réserve aussi les métadonnées aux administrateurs, modérateurs du dépôt, propriétaires/collaborateurs du domaine (y compris L0) et membres des équipes globales liées au domaine ou à l’artefact. Aucun utilisateur ne peut télécharger les fichiers, POM, signatures ou sommes de contrôle. Les personnes autorisées peuvent consulter le catalogue et le contenu analysé de `maven-metadata.xml` ; les autres ne découvrent pas les artefacts ou versions concernés dans le catalogue, la recherche, les répertoires, les API de versions ou les statistiques. Le XML partagé omet les versions masquées et ajuste latest/release sans modifier le fichier stocké.

Les verrous couvrent les fichiers associés arbitraires et les SNAPSHOT horodatés. Les fichiers gelés n’expirent pas et ne sont pas récupérés depuis les miroirs. Les métadonnées partagées ne peuvent pas être remplacées lorsqu’une version est verrouillée ; la dépréciation permanente de l’artefact et la reconfiguration/suppression du dépôt sont aussi bloquées. Les détails exposent `locks`, `moderator`, `member` et `version_locked`. Une écriture refusée renvoie `423` avec `X-Renop-Error-Code: resource_locked` ; une lecture refusée renvoie `404`.

La suppression d’une version valide chaque segment de coordonnées avant toute modification du stockage ; les séparateurs de chemin et les alias de répertoires à points sont refusés.
