---
title: API du centre de messages
order: 7
category: Référence API
description: Notifications, compteurs, actions de workflow et annonces administrateur
---

# API du centre de messages

Toutes les routes exigent une authentification. Les réponses utilisent protobuf par défaut et ne sont jamais mises en
cache. Un API Token requiert `messages:read`; la composition exige aussi `admin:notifications` et le rôle administrateur.

## 1. Lister ou effacer les messages

- **Lister** : `GET /api/messages?limit=30&cursor=...`
- **Effacer les messages résolus** : `DELETE /api/messages`
- `limit` vaut 1 à 100. `cursor` est le `next_cursor` opaque de la page précédente.
- L’effacement conserve tout message dont l’action de workflow est encore `pending`.

### Exemple de réponse décodée

```json
{
  "messages": [
    {
      "id": "00000000-0000-4000-8000-000000000001",
      "kind": "announcement",
      "severity": "info",
      "title": "Maintenance",
      "body": "Maintenance starts at 02:00 UTC.",
      "action_status": "",
      "created_at": 1787731200000,
      "read_at": 0
    }
  ],
  "unread_count": 1,
  "next_cursor": ""
}
```

## 2. Lire le nombre de messages non lus

- **Chemin** : `GET /api/messages/unread-count`
- **Réponse décodée** : `{"unread_count":3}`

## 3. Marquer ou supprimer

### Un message

- **Marquer comme lu** : `POST /api/messages/:id/read`
- **Supprimer** : `DELETE /api/messages/:id`
- Le message d’un autre compte renvoie `404`. Une action encore en attente renvoie `409`.

### Tous les messages

- **Tout marquer comme lu** : `POST /api/messages/read-all`
- La réponse indique le nombre de lignes modifiées.

## 4. Envoyer une annonce administrateur

- **Chercher les destinataires** : `GET /api/messages/admin/users?q=alice` renvoie au plus huit noms.
- **Envoyer** : `POST /api/messages/admin`
- Utilisez `all: true` pour tous les comptes ou fournissez des `recipients` exacts. Le serveur borne le titre, le corps,
  la sévérité et le nombre de destinataires.

```json
{
  "recipients": ["alice", "bob"],
  "all": false,
  "severity": "warning",
  "title": "Scheduled maintenance",
  "body": "The service will restart at 02:00 UTC."
}
```

Les invitations et résultats système sont créés par leur service. Une exclusion d’équipe indique le dépôt et le paquet
ou domaine Maven, mais ne révèle volontairement pas le membre ayant effectué l’action.
