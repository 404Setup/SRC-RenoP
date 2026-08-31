---
title: Intégration de l’API HTTP
order: 19
category: Référence API
description: Choix de l’API, types protobuf, identifiants, erreurs, tentatives et compatibilité des clients
---

# Intégration de l’API HTTP

RenoP sert l’API de gestion et plusieurs protocoles de paquets depuis la même origine. Choisissez d’abord la famille
d’API, puis le type de média et l’identifiant ; tous les endpoints ne sont pas une API REST JSON.

## Choisir la bonne surface d’API

| Surface                      | Chemins courants                            | Client prévu                                    |
|:-----------------------------|:--------------------------------------------|:------------------------------------------------|
| API de gestion et navigateur | `/api/...`                                  | Interface RenoP, administration, automatisation |
| Maven et fichiers            | `/{repo}/{path}`                            | Maven, Gradle, clients HTTP d’artefacts         |
| Registre sparse Cargo        | `/{repo}/config.json`, `/{repo}/api/v1/...` | Cargo et outils compatibles                     |
| Registre npm                 | `/{repo}/{package}`, `/{repo}/-/...`        | Clients npm compatibles                         |
| Distribution Docker/OCI      | `/v2/...`, `/v2/token`                      | Docker, Podman, clients OCI                     |
| Aperçus de documentation     | `/javadoc/...`, `/cargodoc/...`             | Navigateur après autorisation du dépôt          |

N’ajoutez pas `/api` à une URL de client natif. N’appliquez pas non plus les méthodes ou erreurs d’un protocole de
paquet
à l’API de gestion.

## Utiliser la représentation déclarée

La plupart des requêtes et réponses de gestion utilisent `application/x-protobuf`. Les schémas OpenAPI décrivent des
champs logiques ; leurs exemples ne rendent pas l’endpoint compatible JSON. Utilisez les messages
`proto/api/v1/api.proto` de la même version de RenoP.

Pour un corps protobuf, définissez explicitement :

```http
Content-Type: application/x-protobuf
Accept: application/x-protobuf
```

Le health check et certaines erreurs sont en texte. Cargo, npm et Docker/OCI utilisent les formats JSON ou binaires de
leur protocole. Suivez la documentation de l’endpoint au lieu de déduire le format du suffixe.

## Choisir l’identifiant selon l’appelant

| Identifiant            | Usage                                              | Restriction importante                                |
|:-----------------------|:---------------------------------------------------|:------------------------------------------------------|
| Cookie `renop_session` | Navigateur interactif et sécurité privée du compte | HttpOnly ; ne pas l’extraire pour des scripts         |
| Jeton API Bearer       | Automatisation de gestion et routes compatibles    | Droits effectifs limités par le compte et les équipes |
| HTTP Basic             | Clients de paquets et flux d’envoi désignés        | Ne remplace pas partout la session ou le Bearer       |
| Bearer Docker          | Opérations Distribution Docker/OCI                 | Reçu après challenge et échange de jeton              |

Le secret d’un jeton n’est affiché qu’à sa création. Stockez-le dans un gestionnaire, définissez expiration, cibles et
scopes minimaux, puis révoquez-le à la fin. Les identifiants en query string et `Authorization: Session` sont refusés.

## Construire correctement l’URL de base

Utilisez une origine HTTPS canonique en production. Le reverse proxy doit conserver `Host` et le schéma pour que
cookies,
redirections, challenges Docker et URL générées visent le service public.

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

Une réponse réussie contient `"UP"`.

Cette route teste l’accès, pas la capacité de la base ou du stockage à valider une écriture. Ajoutez une vérification
authentifiée séparée si le déploiement doit tester ces dépendances.

## Traiter la réponse dans un ordre stable

1. Lire le statut HTTP.
2. Examiner le `Content-Type`.
3. Lire `X-Renop-Error-Code` s’il existe.
4. Décoder le corps avec le décodeur du protocole concerné.
5. Journaliser heure et contexte expurgé, jamais l’identifiant.

Les erreurs de gestion peuvent être en texte court. Docker Distribution, Cargo et npm conservent leurs erreurs
structurées.
Ne branchez pas le code sur une phrase anglaise complète.

## Associer le statut au comportement du client

| Statut      | Action du client                                                                                |
|:------------|:------------------------------------------------------------------------------------------------|
| `200`–`204` | Décoder selon le type documenté ; un corps vide réussi peut être valide                         |
| `202`       | Opération acceptée mais pas forcément visible ; une revue peut rester en attente                |
| `302`       | Suivre uniquement pour un téléchargement documenté, tel qu’une URL S3 présignée autorisée       |
| `400`       | Corriger la requête ; la répétition automatique reproduit généralement l’erreur                 |
| `401`       | Vérifier que le type d’identifiant est accepté avant de le renouveler                           |
| `403`       | Ne pas répéter aveuglément ; scopes, cibles, droits, équipe, règle ou debug doivent changer     |
| `404`       | Vérifier chemin et visibilité ; une donnée privée ou masquée peut être volontairement cachée    |
| `409`       | Relire l’état avant de modifier une opération immuable ou concurrente                           |
| `413`       | Réduire le corps si c’est valide, sinon corriger les limites proxy/serveur                      |
| `429`       | Respecter l’attente, ajouter du jitter et réduire la concurrence                                |
| `5xx`       | Ne répéter que les opérations sûres et bornées ; conserver l’erreur et vérifier les dépendances |

## Ne répéter que si la sémantique l’autorise

GET et HEAD sont généralement sûrs après une erreur de transport. Pour une écriture, déterminez l’idempotence et si le
serveur a pu valider avant la coupure. Utilisez un backoff exponentiel borné, avec jitter et délai total.

Ne changez pas silencieusement de version, ne supprimez pas de données et n’élargissez pas les droits pour répéter une
publication immuable. Pour les envois segmentés, reprenez avec l’état défini par le protocole.

## Respecter la pagination et les filtres de chaque endpoint

Les listes n’utilisent pas toutes le même curseur. Appliquez les paramètres documentés, conservez les identifiants
stables et arrêtez lorsque la page indique la fin. Un filtre d’interface ne modifie ni autorisation ni visibilité.

## Garder les contrats d’une même version

Générez les clients avec `web/assets/openapi.yaml` et `proto/api/v1/api.proto` du même commit ou release que le serveur.
Un champ OpenAPI représente un champ protobuf logique, pas forcément du JSON. Maven, Cargo, npm et Docker doivent rester
configurés avec leurs protocoles natifs.

Avant mise à niveau, testez hors production : connexion, autorisation par jeton, liste des dépôts, lecture et écriture
de
chaque format, pagination, décodage d’erreur et reverse proxy.

## Checklist d’intégration

- [ ] Bonne famille d’API et bon chemin de dépôt.
- [ ] Origine HTTPS, hôte et schéma du proxy canoniques.
- [ ] Types de média explicites.
- [ ] Type d’identifiant autorisé pour la route.
- [ ] Scopes, cibles, expiration et droits minimaux et actuels.
- [ ] Statut traité avant le texte du corps.
- [ ] Tentatives bornées, avec jitter et sûres pour l’opération.
- [ ] Cookies, mots de passe, jetons et URL signées expurgés des journaux.
- [ ] OpenAPI et protobuf correspondent au serveur déployé.
- [ ] Test natif de bout en bout avant déploiement.

Voir [API d’authentification](./authentication.md), [Jetons API et utilisateurs](./tokens.md) et
[Dépannage](../guides/troubleshooting.md).
