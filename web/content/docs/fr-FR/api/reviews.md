---
title: API de validation
order: 13
category: Référence API
description: Demandes de transfert de propriété, filtrage et décision unique hors messagerie
---

# API de validation

L'API de validation sépare les transferts de propriété du centre de messages. Les tâches sont persistantes, paginées
et ne peuvent recevoir qu'une seule décision. Elles couvrent les images Docker, les paquets npm, les crates Cargo,
les artefacts Maven et les domaines de publication Maven.

## Portée et identifiants

Les routes de validation acceptent uniquement une session de navigateur authentifiée. Les identifiants Basic et les
API Token Bearer ne peuvent ni créer, ni lister, ni décider, ni annuler une tâche. Le menu du compte ouvre le même
parcours à l'adresse `/account/reviews`.

La vue de validation contient les tâches des équipes où le compte courant est T3 ou T4. La vue des demandes repose sur
l'identité immuable du compte et conserve donc les tâches antérieures à un changement de nom d'utilisateur.

## Règles de transfert

Le demandeur doit posséder le projet ou le domaine avec un niveau effectif L4, ou disposer de l'administration actuelle
du dépôt ou du système. Pour un transfert vers une équipe globale, il doit aussi en être membre. Un gestionnaire T3/T4
de l'équipe ou un administrateur système décide la demande ; un demandeur disposant de ce droit peut traiter sa tâche.

Le transfert modifie uniquement le rattachement de propriété. Les membres du paquet ne sont ni copiés ni supprimés.
Un transfert direct entre deux équipes est refusé : il faut d'abord rendre un projet admissible à la propriété
personnelle, puis soumettre une nouvelle demande.

Les images Docker avec espace de noms et les paquets npm avec portée ne peuvent pas redevenir personnels, car leur nom
réserve le préfixe immuable de l'équipe. Les ressources provenant d'un miroir ne sont pas transférables.

## Lister les tâches

GET /api/reviews renvoie une page bornée. `view` accepte `reviewer` ou `requested` ; `status` accepte `pending`,
`approved`, `rejected`, `cancelled` ou `all`. Le filtre facultatif `types`, séparé par des virgules, accepte les cinq
types de ressource. `limit` est compris entre 1 et 100 et `offset` ne peut pas être négatif.

La réponse contient `tasks`, `total`, `limit`, `offset` et la valeur résolue de `view`. Chaque tâche conserve les
préfixes source et cible, le nom du demandeur, les horodatages, l'état et les métadonnées de décision.

## Demander un transfert

POST /api/reviews/super-team-transfers accepte `resource_type`, `repository`, `resource_key` et
`target_team_prefix`. Un domaine Maven omet `repository`. Un artefact Maven utilise une clé
`groupId:artifactId`. Une cible vide demande un retour à la propriété personnelle.

Une ressource ne peut avoir qu'un transfert de propriété en attente, quelle que soit la cible demandée. La création
renvoie `201 Created`, la tâche et son emplacement API.

## Décider ou annuler

POST /api/reviews/{id}/decision accepte `approved` ou `rejected`. Un refus exige un motif non vide de 512 caractères
au maximum ; un motif joint à une approbation est ignoré. L'approbation revérifie le rattachement et applique le
transfert dans la même transaction de base de données que la décision.

DELETE /api/reviews/{id} permet uniquement au demandeur d'annuler une tâche encore en attente. Une comparaison et mise
à jour sur l'état en attente garantit que toute décision concurrente ultérieure reçoit un conflit sans modifier la
ressource.

## Gestion des erreurs

Les échecs exposent un `X-Renop-Error-Code` stable. `400` signale un filtre, un identifiant ou une décision incorrecte.
`403` signale l'absence de propriété, d'appartenance à l'équipe cible ou d'autorité de validation. `404` signifie que
la tâche est absente. `409` couvre une demande en double, une tâche terminée, une propriété modifiée ou un transfert
interdit.

Les clients doivent traduire le code enregistré et ne jamais afficher directement le corps de la réponse.
