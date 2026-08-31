---
title: Sauvegarde, restauration et migration
order: 5
category: Déploiement
description: Sauvegardes cohérentes, répétitions de restauration, migration de backend et validation de reprise
---

# Sauvegarde, restauration et migration

Une sauvegarde RenoP n’est complète que si configuration, règles de dépôts, état de la base et artefacts non
reconstructibles peuvent être restaurés ensemble. Copier uniquement `index.json` ou un bucket S3 ne suffit pas.

## Classer les données

| Donnée                      | Emplacement courant                         | Rôle à la restauration                                              |
|:----------------------------|:--------------------------------------------|:--------------------------------------------------------------------|
| Configuration principale    | `config.yaml` ou `RENOP_CONFIG`             | Écoute, base, proxy, sécurité, aperçus, mise à jour                 |
| Définition des dépôts       | `repositories.yaml` ou `RENOP_REPOSITORIES` | Format, visibilité, miroirs, stockage, règles                       |
| Base de données             | `renop.db` ou DSN externe                   | Comptes, droits, sessions, jetons, équipes, revues, audit, messages |
| Artefacts locaux            | `storage_path`                              | Paquets publiés, envois, cache amont                                |
| Artefacts S3 compatibles    | Bucket et préfixe par dépôt                 | Paquets et cache des dépôts S3                                      |
| Index de fichiers           | `index.json` ou `RENOP_INDEX`               | Instantané de performance, utile mais reconstructible               |
| Secrets TLS et intégrations | Proxy ou gestionnaire de secrets            | Restauration du service public et des intégrations                  |

Les sorties du site, dépendances frontend et caches de build sont reproductibles et ne doivent contenir l’unique copie
d’aucun secret d’exploitation.

## Choisir un point de cohérence

La méthode générale la plus sûre est une sauvegarde à froid : bloquer le trafic, arrêter RenoP proprement, capturer la
base et les backends d’artefacts, copier la configuration puis redémarrer. Elle évite une référence en base vers un
objet
capturé à un autre instant.

Sans interruption, utilisez des sauvegardes transactionnellement cohérentes et des snapshots de stockage rattachés à un
point de reprise commun. Copier seulement le fichier SQLite actif est dangereux si des données validées sont encore
dans le WAL. Deux snapshots lancés presque simultanément ne sont pas nécessairement cohérents.

## Sauvegarder une installation SQLite locale

Arrêtez RenoP avec le gestionnaire qui le démarre. Une fois le processus terminé, copiez la base fermée, les fichiers de
configuration, l’index et tout le stockage local.

```bash
install -d /backup/renop
cp config.yaml repositories.yaml renop.db index.json /backup/renop/
rsync -a storage/ /backup/renop/storage/
```

Adaptez les chemins définis par `RENOP_CONFIG`, `RENOP_REPOSITORIES`, `RENOP_INDEX`, le DSN et `storage_path` ; les
noms ci-dessus sont les valeurs usuelles. Préservez propriétaire, permissions, attributs nécessaires et espace pour les
fichiers temporaires.

## Sauvegarder une base externe

Utilisez le dump logique, la sauvegarde physique ou le snapshot géré pris en charge par le fournisseur. Incluez toutes
les tables RenoP et les métadonnées de migration. Chiffrez les transferts et fichiers, notez les versions du serveur et
du moteur, puis vérifiez avec l’outil de restauration officiel.

Pour MySQL ou PostgreSQL, privilégiez un point cohérent couvrant toutes les tables. Pour ClickHouse, suivez les
exigences
de l’installation et conservez les données nécessaires au journal transactionnel RenoP. Ne tentez pas de reconstruire
les comptes ou équipes à la main après perte de la base.

## Sauvegarder les artefacts locaux et S3

Pour le disque local, copiez la racine complète. Ne filtrez pas par extension : métadonnées, manifests, index,
signatures
et état d’envoi peuvent être aussi importants que l’archive principale.

Pour un stockage compatible S3 :

- Protégez chaque bucket et `key_prefix` utilisé.
- Activez versioning ou réplication si disponible et testez la récupération réelle d’un objet.
- Séparez les identifiants de sauvegarde de ceux utilisés par RenoP.
- Préservez les métadonnées et vérifiez les règles de cycle de vie.
- Gardez le bucket privé sauf nécessité explicite liée aux URL présignées.

Le cache miroir peut souvent être reconstitué, contrairement aux artefacts publiés localement. N’appliquez des
rétentions
différentes qu’après les distinguer de façon fiable.

## Restaurer dans un environnement isolé

Restaurez d’abord sur un hôte ou réseau isolé. Utilisez la même version de RenoP que lors de la sauvegarde, validez le
service, puis réalisez l’éventuelle mise à niveau séparément.

1. Restaurez `config.yaml`, `repositories.yaml`, certificats et secrets avec des permissions strictes.
2. Restaurez la base et vérifiez hôte, identifiants et TLS.
3. Restaurez le disque ou reconnectez exactement le même bucket et préfixe S3.
4. Restaurez `index.json` s’il existe ; sinon laissez RenoP reconstruire l’index.
5. Démarrez sans trafic public et examinez les erreurs.
6. Connectez-vous, listez les dépôts et testez des lectures représentatives.
7. Publiez puis supprimez un paquet jetable avec un jeton minimal.
8. Rouvrez le trafic après validation des droits, miroirs, revues, quotas, aperçus et audit.

Après un incident de sécurité, ne réactivez pas automatiquement les anciennes sessions et jetons. Révoquez-les et
faites tourner les secrets de base, stockage, OAuth, SMTP, proxy et signature selon le périmètre.

## Migrer le backend d’un dépôt

Utilisez la migration de gestion RenoP afin de sérialiser les opérations avec le changement de backend. Ne modifiez pas
les répertoires physiques, ne copiez pas les objets actifs derrière RenoP et ne changez pas la configuration avant
vérification de la copie.

Avant migration, relevez paquets, versions, octets, règles, source, destination et capacité. Après migration, comparez
listes et hashes représentatifs, testez lecture et écriture natives, puis gardez la source en lecture seule jusqu’à la
fin
de la période d’acceptation.

## Répéter la reprise

Définissez RPO et RTO pour le service complet. Restaurez périodiquement la dernière sauvegarde dans un environnement
vide
et consignez :

- heures de début et fin de sauvegarde ;
- versions de RenoP, de la base et du stockage ;
- durée et étapes manuelles ;
- résultats lecture/écriture pour chaque format ;
- objets absents, erreurs de droits, DNS ou certificats obsolètes et actions correctives.

Une sauvegarde jamais restaurée reste une hypothèse. Reliez le runbook à la
[Checklist de mise en production](./production-checklist.md) et conservez-en une copie hors ligne.
