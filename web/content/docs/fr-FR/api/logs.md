---
title: Journaux globaux et activité
order: 13
category: Référence API
description: Consulter les diagnostics et filtrer les opérations des comptes
---

# Journaux globaux et activité

## Accès

Ouvrez **Journaux globaux** dans le tableau de bord administrateur. Le compte doit être administrateur système et un API
Token doit aussi posséder `admin:audit`. Un modérateur de dépôt ne suffit pas. La vue personnelle reste limitée au
compte courant ; les administrateurs peuvent consulter un autre compte.

L’initiateur est le compte à l’origine de la demande ; l’opérateur est le compte ou la tâche qui l’exécute. Les
opérations directes et les lignes historiques utilisent l’opérateur comme initiateur. Les deux identités suivent les
mêmes restrictions de masquage et de filtre personnel. La source reste dans `trigger`. Le renommage et la fermeture
traitent les deux identités.

```http
GET /api/auth/logs?kind=system&severity=error&trigger=http&page=1&page_size=20
GET /api/auth/profile/audit-logs?action=LOGIN&from=1788739200000&until=1788825599999
GET /api/auth/users/alice/audit-logs?operator=admin&trigger=web
```

## Filtres

Les conditions se combinent avec AND. Les textes correspondent exactement ; une valeur vide désactive le filtre. Les
dates utilisent l’heure locale du navigateur puis des millisecondes Unix. Une valeur invalide renvoie `400` et
`LOG_FILTER_INVALID`. Action et origine sont limitées à 64 octets, compte et opérateur à 255 octets.

| Paramètre           | Valeurs                                                                               |
|---------------------|---------------------------------------------------------------------------------------|
| `kind`              | `audit`, `system`                                                                     |
| `action`            | Identifiant exact de l’action, par exemple `LOGIN`, `SYSTEM_LOG`, `SYSTEM_HTTP_ERROR` |
| `operator`          | Nom exact ; la vue personnelle accepte aussi `@administrator`                         |
| `initiator`         | Nom exact ; la vue personnelle accepte aussi `@administrator`                         |
| `username`          | Compte concerné, vue globale uniquement                                               |
| `trigger`           | Origine exacte `web`, `api`, `http`, `system`, `unknown`                              |
| `severity`          | `info`, `warning`, `error`                                                            |
| `from`, `until`     | Millisecondes Unix inclusives ; chaque borne est facultative                          |
| `page`, `page_size` | Valeurs par défaut `1`, `20` ; taille `1`–`200` ; décalage maximal `1000000`          |

## Visibilité

Le protobuf `AuditLogList` reçoit les champs supplémentaires `kind`, `trigger`, `initiator`, `severity`. Les vues
personnelles et par compte restent de type `audit`. Les autres opérateurs sont masqués comme Administrateur ;
`operator=@administrator` sélectionne ce groupe. Toute tentative de filtrer par un autre nom, même inexistant, renvoie
`400` et ne révèle aucune identité masquée. Le filtre `username` ne peut pas élargir la portée du compte.

## Diagnostics système

La capture commence après le démarrage des services. Les erreurs HTTP serveur sont enregistrées sans renvoyer les
détails au public. Les motifs courants de mots de passe, en-têtes d’authentification, jetons et identifiants URL sont
masqués ; chaque entrée est limitée à 4096 octets. Les producteurs existants déduisent l’origine de la méthode
d’authentification ; les lignes migrées utilisent `unknown`. Le niveau des messages non structurés est déduit de mots
clés ; les erreurs HTTP structurées utilisent `error`.

## Conservation et écriture

Un consommateur écrit les entrées en série. L’arrêt normal restaure la sortie originale et vide la file de 500 entrées.
En cas de saturation, l’activité conserve son écriture synchrone de secours ; les messages système restent dans la
sortie originale, incrémentent les échecs et produisent un avis de saturation quand la capacité revient. Les erreurs de
persistance utilisent uniquement la sortie originale pour éviter une récursion. L’activité conserve le budget
`audit_log` ; le système a un budget séparé, au plus 30 jours et 10000 lignes, ou les limites configurées plus basses.
Le nettoyage a lieu toutes les dix minutes. Les règles de fermeture des comptes et de purge définitive s’appliquent
aussi aux diagnostics associés. Aucun historique de processus n’est importé.
