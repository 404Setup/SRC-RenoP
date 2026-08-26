---
title: API токенов и пользователей
order: 3
category: Справочник API
description: Жизненный цикл API-токенов с детальными правами и управление пользователями
---

# API токенов и пользователей

Для управления секретами требуется HttpOnly cookie браузера `renop_session`:

- `GET /api/auth/profile/api-tokens/scopes` — доступные текущему аккаунту области.
- `GET /api/auth/profile/api-tokens` — метаданные без секретов и лимит в 50 токенов.
- `POST /api/auth/profile/api-tokens` — создание; секрет `rnp_pat_...` возвращается один раз.
- `DELETE /api/auth/profile/api-tokens/{token_id}` — немедленный отзыв.

Автоматизация использует `Authorization: Bearer <token>`. Basic ограничен протоколами пакетов. Административные
операции с пользователями остаются в `/api/tokens` и требуют область `admin:users`.
