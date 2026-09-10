---
title: API des tickets
order: 13
category: Référence API
description: Les tickets réunissent retours, suggestions, signalements, transferts de propriété et approbations de publication dans un parcours persistant. Le centre de tickets remplace le centre de validation en conservant les identifiants et les publications en attente.
---

# API des tickets

Les tickets réunissent retours, suggestions, signalements, transferts de propriété et approbations de publication dans
un parcours persistant. Le centre de tickets remplace le centre de validation en conservant les identifiants et les
publications en attente.

## Portée et identifiants

Chaque route exige un cookie de navigateur actif `renop_session`. Les identifiants Basic, les API Token Bearer et les
jetons de session sans ce cookie sont refusés. Ouvrez `/account/tickets` ; les anciens liens `/account/reviews` y sont
redirigés.

Les modérateurs voient tous les états des tickets de leurs dépôts, même pendant l’étape d’équipe. Les administrateurs
système voient toutes les portées. Les membres T3/T4 traitent seulement les transferts et créations affectés à leur
équipe. L’approbation d’équipe précède toujours celle du dépôt. L’historique suit l’identité immuable du demandeur.

Les agents autorisés voient les identités de leurs collègues dans le ticket. Le demandeur ne reçoit jamais `assignee`,
`escalated_by` ni `decided_by`. Le compte signalé ne peut ni lire ni traiter son signalement, même administrateur. La
cible ne reçoit jamais l’identité du demandeur ni l’accès au ticket.

Les rappels suivent l’étape courante. Le résultat final est envoyé au demandeur par message et, si activé, par courriel
dans sa langue. La cible reçoit un avis distinct uniquement si le signalement se termine avec `upheld` : ressource et
résultat, sans identifiant de ticket, demandeur, agent ou texte privé. Rejet et retrait ne l’avertissent pas. Les
identifiants des scènes de courriel restent compatibles.

Les événements d'audit des signalements omettent les identités de compte, d'agent, de session et d'adresse IP ;
l'attribution reste dans les tickets autorisés.

## Envoyer un retour, une suggestion ou un signalement

POST /api/tickets accepte `kind` (`feedback`, `suggestion` ou `report`), `title` (1–160 caractères), `body` (1–8000
caractères) et `repository` facultatif. Le JSON est limité à 48 KiB. Chaque compte peut avoir 16 demandes de support en
attente et en créer 24 par période de 24 heures ; tous les parcours partagent une limite globale de 4096 tâches en
attente.

Un signalement exige aussi `target` : `format`, `repository`, `name` et `version` facultatif. Formats : `user`,
`superteam`, `maven-domain`, `maven`, `cargo`, `npm`, `docker`. Les ressources globales omettent le dépôt. Un paquet
doit correspondre au format du dépôt et être actuellement lisible. Il est possible de signaler un autre utilisateur, un
paquet ou une version visible, un domaine ou une équipe ; ses propres ressources, les ressources masquées et les
doublons en attente sont refusés.

La création renvoie `201`, le ticket et `Location`. Les propriétaires ciblés sont enregistrés par identifiants
immuables. Enregistrer `upheld` ne bannit pas automatiquement un compte et ne verrouille pas un paquet : appliquez
d’abord la mesure de modération existante avant de confirmer une sanction.

```json
{"kind":"report","title":"Package report","body":"Please investigate this version.","target":{"format":"npm","repository":"npm","name":"@platform/tool","version":"1.0.0"}}
```

## Prendre en charge, escalader et résoudre

POST /api/tickets/{id}/action accepte `action` : `claim`, `release`, `escalate`, `process`, `complete` ou `close`. Il
faut prendre en charge le ticket avant de le traiter ou de décider. Cette prise est atomique ; les autres agents gardent
la lecture, sans pouvoir le traiter. Un administrateur système peut utiliser `force: true` pour reprendre le ticket d’un
modérateur, mais pas celui d’un autre administrateur qui ne l’a pas libéré.

L’escalade libère le ticket et réserve sa prochaine prise aux administrateurs système. Un administrateur peut
l’escalader vers un autre administrateur. Trois escalades au maximum sont permises. Après la troisième, le prochain
agent doit terminer : libération, escalade et reprise forcée sont désactivées. L’approbation d’équipe libère
l’affectation pour l’étape du dépôt.

Pour le support, `process` marque le traitement sans notification finale, `complete` termine et `close` ferme. Ces
actions exigent `response` non vide, de 4096 caractères au maximum. Retours et suggestions utilisent
`outcome: resolved`, les signalements `upheld` ou `dismissed`, et la fermeture enregistre `closed`. Le JSON d’action est
limité à 24 KiB. Les publications et transferts utilisent ensuite la route de décision ci-dessous.

La reprise suit le rôle actuel de l’agent : une promotion protège immédiatement un administrateur, tandis que le retrait
de ses droits permet une reprise avant la limite d’escalades.

GET /api/tickets/{id} renvoie le détail autorisé et un tableau `actions` calculé selon les droits et l’affectation
actuels, à utiliser pour les commandes. Les conflits renvoient `409` avec `ticket_claim_required`, `ticket_occupied` ou
`ticket_escalation_limit`.

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

Pour npm, un membre T2 commence toujours par l’approbation T3/T4 de l’équipe, sans réserver le nom. Si le dépôt examine
les créations, cette approbation fait avancer la même tâche vers un modérateur ; sinon elle crée le paquet. L’étape
finale revérifie les droits du dépôt et l’appartenance actuelle à l’équipe avant d’attribuer L4. Avec `new_packages`,
les versions suivantes sont publiées normalement. Avec `every_version`, RenoP masque aussi chaque tarball et conserve
un manifeste ainsi que des dist-tags bornés jusqu’à l’approbation. Le contenu miroir est exclu.

Une publication Cargo stocke et masque l’archive du crate sans modifier l’index sparse ni le catalogue public.
L’approbation ajoute la version immuable aux deux ensembles de métadonnées avant de rendre l’archive accessible ; le
rejet supprime l’archive masquée. Avec `new_packages`, le crate reste nouveau jusqu’à l’approbation de sa première
version visible. Les crates issus d’un miroir ne sont pas examinés.

Pour Docker, une création T2 suit les mêmes étapes ordonnées d’équipe puis, si nécessaire, de dépôt que npm. L’étape
finale revérifie les conflits locaux ou amont, les droits du dépôt et l’appartenance actuelle à l’équipe avant de
réserver l’image. Avec `new_packages`, les manifestes suivants sont publiés normalement. Avec `every_version`, chaque
manifeste exact reste un fichier virtuel
borné jusqu’à l’approbation ; sa référence et son tag n’entrent pas dans le catalogue, de sorte qu’un nouveau tag ne
masque pas un tag existant du même digest. L’enregistrement du manifeste, des blobs, du tag et de la décision est
atomique.

## Lister les tâches

GET /api/tickets renvoie une page bornée. `view` accepte `reviewer` ou `requested` ; `status` accepte `unprocessed`
(défaut), `in_progress`, `processed`, `closed`, `completed` ou `all`. `limit` vaut 1–100 et `offset` est positif ou nul.
Le filtre `types`, séparé par des virgules, accepte les types de parcours ainsi que `support`, `user`, `superteam`,
`maven-domain`, `maven`, `cargo`, `npm`, `docker`.

`ticket_status` représente le cycle commun. Le champ historique `status` conserve le résultat du parcours : `pending`,
`approved`, `rejected` ou `cancelled`. Les anciens enregistrements reçoivent leur état de ticket à la première prise en
charge.

La réponse contient `tasks`, `total`, `limit`, `offset` et la valeur résolue de `view`. Chaque tâche conserve les
préfixes source et cible, l’équipe chargée de l’étape courante, le nom du demandeur, les horodatages, l'état et les
métadonnées de décision. Un `review_team_prefix` non vide affecte la tâche aux membres T3/T4 de cette équipe. Son
effacement avec l’état `pending` indique qu’une création T2 attend maintenant l’examen du dépôt.
Une publication fournit aussi `resource_version`, `file_count`, `total_size` et l'heure du dernier fichier.
La création explicite npm/Docker utilise la valeur réservée `@create` dans `resource_version` et expose sa requête JSON
bornée par la même API de fichiers.

## Demander un transfert

POST /api/tickets/super-team-transfers accepte `resource_type`, `repository`, `resource_key` et
`target_team_prefix`. Un domaine Maven omet `repository`. Un artefact Maven utilise une clé
`groupId:artifactId`. Une cible vide demande un retour à la propriété personnelle.

Une ressource ne peut avoir qu'un transfert de propriété en attente, quelle que soit la cible demandée. La création
renvoie `201 Created`, la tâche et son emplacement API.

## Fichiers à examiner

GET /api/tickets/{id}/files renvoie au plus 256 chemins relatifs, avec identifiant stable, taille, date d'envoi et
indicateur de fichier essentiel. GET /api/tickets/{id}/files/{file_id} diffuse un fichier masqué. Seuls le demandeur,
un membre T3/T4 de l’équipe actuellement affectée, un modérateur du dépôt concerné ou un administrateur système
utilisant une session de navigateur peuvent y accéder.

Le centre web emploie au plus quatre téléchargements adaptatifs et réessaie chaque échec deux fois. Si tout réussit,
il crée dans le navigateur une archive ZIP conforme aux chemins du dépôt. Sinon, il ouvre séparément les fichiers
essentiels au lieu de produire une archive incomplète.

## Décider ou annuler

L’agent courant doit avoir pris en charge cette étape du ticket. POST /api/tickets/{id}/decision accepte `approved` ou
`rejected`. L’approbation d’une création T2 termine la création
ou renvoie la même tâche avec l’état `pending` et un `review_team_prefix` vide lorsque le dépôt doit encore l’examiner.
Le refus d'un transfert exige un motif non vide de
512 caractères au maximum. Le refus d'une publication exige `reason_code` parmi `invalid_metadata`, `quality`,
`policy_violation`, `copyright`, `malware` et `custom`. Un motif personnalisé est limité à 505 caractères.
L'approbation enregistre les métadonnées de version du moteur avant d'exposer les fichiers ; le refus supprime les
fichiers masqués.

DELETE /api/tickets/{id} permet au demandeur de retirer une demande de support, de transfert ou de restauration Maven en
attente. Le ticket est fermé et les décisions ultérieures sont bloquées. Les publications ne peuvent pas être annulées
par cette route. Des décisions finales concurrentes ne peuvent pas modifier deux fois la ressource.

## Gestion des erreurs

Les échecs exposent un `X-Renop-Error-Code` stable. `400` signale un filtre, un identifiant ou une décision incorrecte.
`403` signale l'absence de propriété, d'appartenance à l'équipe cible ou d'autorité de validation. `404` signifie que
la tâche ou le fichier est absent. `409` couvre une demande en double, une tâche terminée, une propriété modifiée, un
transfert interdit ou une publication qui reçoit encore des fichiers.

Les clients doivent traduire le code enregistré et ne jamais afficher directement le corps de la réponse.

## Rétablir un paquet Maven revendiqué

`POST /api/tickets/maven-restorations` exige le cookie actif `renop_session` du propriétaire L4 actuel du domaine :

```json
{"resource_type":"maven_artifact","repository":"releases","resource_key":"com.example:demo"}
```

La réponse HTTP `201` contient une tâche `maven_restore` à l’état `pending` ; une demande équivalente en attente renvoie
`409`.
Les modérateurs du dépôt et les administrateurs système voient la tâche. Les routes habituelles de décision et
d’annulation s’appliquent.
L’approbation revérifie atomiquement la revendication, la propriété effective et les restrictions indépendantes avant de
rétablir la publication et le lien avec l’équipe actuelle du domaine.
Une revendication modifiée annule la demande obsolète. Le rejet ou l’annulation conserve la restriction et les
téléchargements.
Les demandes en attente sont limitées à 64 par compte et 4096 au total. Ces tâches ne proposent aucune archive de revue
à télécharger.

Le formulaire de signalement propose des sujets modifiables pour le spam, les abus, les logiciels malveillants, le droit d’auteur, l’usurpation et les autres problèmes. Le bouton est masqué sur son propre profil et le serveur refuse indépendamment les auto-signalements par identifiant immuable. Les sélecteurs de type et de portée partagent les contrôles étiquetés des paramètres.
