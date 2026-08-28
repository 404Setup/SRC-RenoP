---
title: Audit et centre de messages
order: 3
category: Sécurité
description: Actions durables, workflows et frontières de confidentialité
---

# Audit et centre de messages

L’audit et les messages ont des rôles distincts. L’audit indique qui a effectué une action sensible ; le message
présente à l’utilisateur un résultat ou workflow localisé. Tous deux sont durables en base.

## Journaux d’audit

Les actions utilisent des identifiants stables dans un registre backend unique. La validation frontend exige une
traduction de chaque action dans chaque langue.

### Événements enregistrés

- connexions, mots de passe, récupération et méthodes de connexion ;
- création/révocation d’API Token et révocation de sessions ;
- administration des utilisateurs, rôles, dépôts, stockage, proxy et mises à jour ;
- vérification et équipes Maven, cycles d’équipes npm/Cargo/Docker ;
- uploads, suppressions, quarantaine/publication GPG et mutations de paquets.

Une entrée contient sujet, opérateur si nécessaire, méthode, ID public de session, IP, date et détail borné. Rétention et
nombre maximal sont globaux. Seuls les utilisateurs autorisés lisent ou effacent ces données.

## Centre de messages

Les messages prennent en charge pagination, non-lus, lecture individuelle/globale, suppression et actions `pending`.

### Catégories et confidentialité

- **Annonces** : message administrateur ciblé ou global.
- **Workflow** : invitations, résultats GPG et décisions requises.
- **Collaboration** : changements d’équipe et exclusions neutres.
- **Système** : mise à jour disponible et échecs durables ; la progression reste un toast.

Une exclusion indique dépôt et paquet ou domaine Maven, mais pas l’opérateur. Les clés de déduplication empêchent les
vérifications répétées de saturer la boîte. Le compteur apparaît dans le menu et près de l’avatar.
