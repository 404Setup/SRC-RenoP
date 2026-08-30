---
title: API de validation
order: 13
category: Référence API
description: Transferts de propriété et examen indépendant des publications
---

# API de validation

L'API de validation sépare les transferts de propriété et les publications modérées du centre de messages. Les tâches sont persistantes, paginées
et ne peuvent recevoir qu'une seule décision. Elles couvrent les images Docker, les paquets npm, les crates Cargo,
les artefacts Maven et les domaines de publication Maven.

## Portée et identifiants

Les routes de validation acceptent uniquement une session de navigateur authentifiée. Les identifiants Basic et les
API Token Bearer ne peuvent ni créer, ni lister, ni décider, ni annuler une tâche. Le menu du compte ouvre le même
parcours à l'adresse `/account/reviews`.

La vue de validation contient les transferts des équipes où le compte est T3 ou T4, ainsi que les publications des
dépôts où il est modérateur. Les administrateurs système voient toutes les tâches. La vue des demandes repose sur
l'identité immuable du compte et conserve les tâches antérieures à un changement de nom d'utilisateur.

## Règles de transfert

Le demandeur doit posséder le projet ou le domaine avec un niveau effectif L4, ou disposer de l'administration actuelle
du dépôt ou du système. Pour un transfert vers une équipe globale, il doit aussi en être membre. Un gestionnaire T3/T4
de l'équipe ou un administrateur système décide la demande ; un demandeur disposant de ce droit peut traiter sa tâche.

Le transfert modifie uniquement le rattachement de propriété. Les membres du paquet ne sont ni copiés ni supprimés.
Un transfert direct entre deux équipes est refusé : il faut d'abord rendre un projet admissible à la propriété
personnelle, puis soumettre une nouvelle demande.

Les images Docker avec espace de noms et les paquets npm avec portée ne peuvent pas redevenir personnels, car leur nom
réserve le préfixe immuable de l'équipe. Les ressources provenant d'un miroir ne sont pas transférables.

## Règles de publication

Un dépôt Maven peut désactiver l'examen, examiner uniquement la première version d'un nouvel artefact ou examiner
chaque version. L'activation désactive le redéploiement. Les fichiers locaux sont enregistrés, mais restent absents de
l'index public jusqu'à la décision d'un modérateur du dépôt ou d'un administrateur système. Les miroirs sont exclus.

Si les signatures GPG détachées sont obligatoires, leur validation précède l'examen. Les fichiers d'une même version,
y compris les sommes, signatures et métadonnées Maven, rejoignent une tâche unique. Une période de stabilisation de
cinq secondes après le dernier fichier empêche toute décision pendant l'envoi. Une version approuvée est ensuite
verrouillée contre l'ajout de fichiers.

Une publication npm constitue déjà une transaction complète. RenoP masque son tarball et conserve un manifeste ainsi
que des dist-tags bornés jusqu’à l’approbation, puis enregistre ensemble la version immuable et ses tags. Avec
`new_packages`, une réservation reste nouvelle jusqu’à l’approbation de sa première version visible. Les packuments et
tarballs provenant d’un miroir ne créent jamais de tâche.

Une publication Cargo stocke et masque l’archive du crate sans modifier l’index sparse ni le catalogue public.
L’approbation ajoute la version immuable aux deux ensembles de métadonnées avant de rendre l’archive accessible ; le
rejet supprime l’archive masquée. Avec `new_packages`, le crate reste nouveau jusqu’à l’approbation de sa première
version visible. Les crates issus d’un miroir ne sont pas examinés.

## Lister les tâches

GET /api/reviews renvoie une page bornée. `view` accepte `reviewer` ou `requested` ; `status` accepte `pending`,
`approved`, `rejected`, `cancelled` ou `all`. Le filtre facultatif `types`, séparé par des virgules, accepte les cinq
types de ressource. `limit` est compris entre 1 et 100 et `offset` ne peut pas être négatif.

La réponse contient `tasks`, `total`, `limit`, `offset` et la valeur résolue de `view`. Chaque tâche conserve les
préfixes source et cible, le nom du demandeur, les horodatages, l'état et les métadonnées de décision.
Une publication fournit aussi `resource_version`, `file_count`, `total_size` et l'heure du dernier fichier.

## Demander un transfert

POST /api/reviews/super-team-transfers accepte `resource_type`, `repository`, `resource_key` et
`target_team_prefix`. Un domaine Maven omet `repository`. Un artefact Maven utilise une clé
`groupId:artifactId`. Une cible vide demande un retour à la propriété personnelle.

Une ressource ne peut avoir qu'un transfert de propriété en attente, quelle que soit la cible demandée. La création
renvoie `201 Created`, la tâche et son emplacement API.

## Fichiers à examiner

GET /api/reviews/{id}/files renvoie au plus 256 chemins relatifs, avec identifiant stable, taille, date d'envoi et
indicateur de fichier essentiel. GET /api/reviews/{id}/files/{file_id} diffuse un fichier masqué. Seuls le demandeur,
un modérateur affecté au dépôt ou un administrateur système utilisant une session de navigateur peuvent y accéder.

Le centre web emploie au plus quatre téléchargements adaptatifs et réessaie chaque échec deux fois. Si tout réussit,
il crée dans le navigateur une archive ZIP conforme aux chemins du dépôt. Sinon, il ouvre séparément les fichiers
essentiels au lieu de produire une archive incomplète.

## Décider ou annuler

POST /api/reviews/{id}/decision accepte `approved` ou `rejected`. Le refus d'un transfert exige un motif non vide de
512 caractères au maximum. Le refus d'une publication exige `reason_code` parmi `invalid_metadata`, `quality`,
`policy_violation`, `copyright`, `malware` et `custom`. Un motif personnalisé est limité à 505 caractères.
L'approbation enregistre les métadonnées de version du moteur avant d'exposer les fichiers ; le refus supprime les fichiers masqués.

DELETE /api/reviews/{id} permet uniquement au demandeur d'annuler un transfert encore en attente. Une publication ne
peut pas être annulée par cette route. Une comparaison et mise à jour sur l'état en attente garantit qu'une décision
concurrente ultérieure reçoit un conflit sans modifier la ressource.

## Gestion des erreurs

Les échecs exposent un `X-Renop-Error-Code` stable. `400` signale un filtre, un identifiant ou une décision incorrecte.
`403` signale l'absence de propriété, d'appartenance à l'équipe cible ou d'autorité de validation. `404` signifie que
la tâche ou le fichier est absent. `409` couvre une demande en double, une tâche terminée, une propriété modifiée, un
transfert interdit ou une publication qui reçoit encore des fichiers.

Les clients doivent traduire le code enregistré et ne jamais afficher directement le corps de la réponse.
