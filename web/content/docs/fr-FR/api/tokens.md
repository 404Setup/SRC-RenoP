---
title: API des jetons et utilisateurs
order: 3
category: Référence API
description: Cycle de vie des jetons API à permissions fines et gestion des comptes
---

# API des jetons et utilisateurs

La gestion des secrets exige le cookie navigateur HttpOnly `renop_session` :

- `GET /api/auth/profile/api-tokens/scopes` — permissions attribuables au compte actuel.
- `GET /api/auth/profile/api-tokens` — métadonnées sans secret et limite de 50 jetons.
- `POST /api/auth/profile/api-tokens` — création ; le secret `rnp_pat_...` n'est renvoyé qu'une fois.
- `DELETE /api/auth/profile/api-tokens/{token_id}` — révocation immédiate.

Les automatisations utilisent `Authorization: Bearer <token>`. Basic n'est accepté que par les protocoles de paquets.
Les opérations d'administration des comptes restent disponibles sous `/api/tokens` avec la permission `admin:users`.
