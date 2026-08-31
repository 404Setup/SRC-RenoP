---
title: Dépannage
order: 5
category: Guides
description: Méthode fondée sur le statut pour les pannes de démarrage, authentification, proxy, protocole, miroir et stockage
---

# Dépannage

Commencez par le statut HTTP, l’URL exacte, le format et la visibilité du dépôt, puis le type d’identifiant. Ne modifiez
pas plusieurs paramètres en même temps : un client de paquets peut masquer la réponse du serveur par un message
générique.

## Rassembler les éléments minimaux

Avant tout redémarrage ou suppression, notez :

- version et heure de démarrage de RenoP ;
- heure, méthode, URL expurgée et statut de la requête ;
- nom, format, visibilité du dépôt, miroir et revue éventuels ;
- client, version, commande et journal détaillé sans secrets ;
- lignes serveur correspondantes et `X-Renop-Error-Code` s’il existe ;
- accès à la base et au stockage, espace libre et dernières modifications.

Ne publiez jamais cookies de session, secrets de jeton, mots de passe, clés S3/OAuth ou en-têtes d’autorisation
complets.

## Le processus ne démarre pas

Vérifiez d’abord les chemins et le répertoire de travail. Les chemins relatifs de `config.yaml`, `repositories.yaml`,
SQLite, `index.json` et du stockage dépendent de l’environnement du service, parfois différent du terminal interactif.

Les causes courantes sont un port occupé, un YAML invalide, un DSN inaccessible, des droits d’écriture absents, des
fichiers TLS invalides ou un compte de service sans accès aux secrets. `RENOP_DEFAULT_ADMIN_PASSWORD` initialise le
premier compte ; il ne réinitialise pas un administrateur existant.

## Le health check répond mais l’application échoue

```bash
curl -i https://packages.example.com/api/status/health
```

`"UP"` confirme uniquement que la route répond. Il ne teste ni connexion, ni écriture en base, ni stockage, miroir ou
règle de publication. Effectuez ensuite une requête authentifiée et une opération sur un paquet jetable.

Si l’interface signale une nouvelle version, rechargez-la avant d’analyser les erreurs protobuf ou de route. Un cache de
proxy ou navigateur peut servir un JavaScript d’une autre version.

## Lire le statut avant le texte

| Statut | Premières vérifications                                                                                  |
|:-------|:---------------------------------------------------------------------------------------------------------|
| `400`  | Protobuf/JSON invalide, chemin ou nom incorrect, champ obligatoire absent, opération non prise en charge |
| `401`  | Identifiant absent, expiré, mal formé ou interdit ; cookie non renvoyé via HTTPS/proxy                   |
| `403`  | Droit du compte, scope/cible du jeton, niveau d’équipe, visibilité, mode debug ou rôle de revue          |
| `404`  | Mauvais chemin, ressource masquée, version absente, échec miroir ou donnée privée volontairement cachée  |
| `409`  | Version/tag immuable, réservation existante, transition ou décision concurrente                          |
| `413`  | Limite d’envoi du proxy ou serveur ; vérifier taille et buffering                                        |
| `429`  | Limitation de débit ou concurrence ; respecter l’attente et réduire le parallélisme                      |
| `5xx`  | Base, stockage, amont, signature, extraction ou erreur interne ; lire les journaux                       |

Les phrases en texte brut sont destinées aux humains. Utilisez le statut, le corps natif du protocole et l’en-tête
stable
lorsqu’il est fourni.

## Authentification et sessions navigateur

L’interface utilise le cookie HttpOnly `renop_session`. Les endpoints privés de sécurité n’acceptent ni mot de passe, ni
Bearer, ni `Authorization: Session`, ni session dans l’URL. Vérifiez HTTPS, le schéma et l’hôte transmis par le proxy,
et le retour du cookie vers la même origine.

Pour l’automatisation, utilisez un Bearer limité. Ses droits effectifs combinent scopes, cibles, droits actuels du
compte,
règles du dépôt et équipe du paquet. Un jeton plus large ne remplace pas un droit de compte ou d’équipe manquant.

## Maven et Gradle

- L’URL doit se terminer par le nom du dépôt RenoP, pas par `/api`.
- Le `<server><id>` Maven doit correspondre exactement à celui de `distributionManagement` ou du dépôt de dépendances.
- Utilisez le nom du compte comme identifiant Basic et un jeton API limité comme mot de passe.
- Le `groupId` doit relever d’un domaine de publication contrôlé avec le niveau d’équipe requis.
- Pour un dépôt signé, envoyez la signature détachée et vérifiez l’enregistrement backend, pas seulement le nom du
  fichier.
- Un redéploiement de release immuable doit échouer ; ne contournez pas la règle en supprimant des fichiers serveur.

## Cargo

- Utilisez une URL sparse contenant le dépôt et terminée par `/`, par exemple
  `sparse+https://packages.example.com/crates/`.
- Exécutez `cargo login --registry <name>` et stockez la valeur complète du jeton RenoP.
- Distinguez `repository:publish`, `package:create`, cycle de vie et gestion d’équipe.
- Si le contrôle du nom amont est indisponible, la première publication échoue sans réserver le nom ; réessayez après
  rétablissement.
- Tant que la revue est en attente, l’archive acceptée n’apparaît ni dans l’index sparse ni dans le catalogue public.

## npm

- Configurez le registre avec le chemin du dépôt, pas seulement l’hôte, et un registre de scope si nécessaire.
- Vérifiez la ligne de jeton du `.npmrc` utilisateur ou CI et ne la commitez pas.
- Réservez le paquet avant la première publication si la règle du dépôt l’exige.
- Les versions sont immuables ; augmenter la concurrence ne résout pas un conflit.
- Pour un paquet miroir, distinguez version amont et paquet local avant de modifier équipes ou dist-tags.

## Docker et OCI

- Connectez-vous à l’hôte du registre ; fournissez image et chemin séparément à `pull`, `push` ou Podman.
- Utilisez un certificat reconnu ; un registre non sécurisé est réservé aux tests isolés.
- Créez ou réservez l’image/namespace avant le premier push si la règle RenoP l’exige.
- Préservez le challenge `/v2/` et l’échange `/v2/token`. Supprimer `Authorization` ou réécrire les chemins casse
  Bearer.
- Identifiez si le rejet concerne blob, manifest ou tag, puis comparez digest et type de média.

## Miroirs, stockage et reverse proxy

Un miss miroir peut venir d’un `404` amont, du cache négatif, d’une allowlist, d’un identifiant expiré, du proxy sortant
ou d’un échec de commit local. Comparez une requête directe depuis l’hôte RenoP à la même requête via RenoP, sans
contourner les droits en production.

Pour S3, vérifiez endpoint, région, path style, bucket, préfixe, horloge, TLS et droits
lecture/écriture/liste/suppression.
Testez les URL présignées depuis le réseau client. En local, contrôlez propriétaire, espace, temporaires et renommage
atomique.

Pour les gros envois, désactivez le buffering, retirez les limites de corps et augmentez les délais. Ne faites confiance
aux en-têtes transférés que depuis les proxys déclarés.

## Escalader avec un cas reproductible

Réduisez le problème à un dépôt, un paquet jetable et une commande. Joignez configuration expurgée, statut attendu et
obtenu, et résultat sans reverse proxy. Listez les tentatives. Ne supprimez pas base, préfixe ou propriété avant d’avoir
conservé les preuves.

Voir [Intégration de l’API HTTP](../api/client-integration.md) et la
[Checklist de mise en production](../deployment/production-checklist.md).
