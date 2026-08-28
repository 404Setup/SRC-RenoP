---
title: Аудит и центр сообщений
order: 3
category: Безопасность
description: Durable записи поведения, workflow notifications и privacy boundaries
---

# Аудит и центр сообщений

Аудит и сообщения решают разные задачи. Аудит отвечает, кто выполнил security action; сообщение показывает
локализованный результат или workflow затронутому пользователю. Оба вида данных durable в базе.

## Журналы аудита

Записи используют stable action IDs из единого backend registry. Frontend validation требует перевод каждой action во
всех locales.

### Записываемые события

- login, password, recovery и изменения login method;
- создание/отзыв API Token и отзыв sessions;
- администрирование user, role, repository, storage, proxy и update;
- Maven domain verification/team и lifecycle команд npm/Cargo/Docker;
- uploads, deletes, GPG quarantine/publication и package mutations.

Entry содержит subject, operator при необходимости, auth method, public session ID, client IP, time и bounded details.
Retention/max rows задаются глобально. Читать или очищать журнал могут только авторизованные пользователи.

## Центр сообщений

Поддерживаются pagination, unread count, individual/all read, deletion и pending workflow actions.

### Категории и privacy

- **Announcements**: Сообщения администратора выбранным или всем аккаунтам.
- **Workflow**: Team invitations, GPG outcomes и действия с решением.
- **Collaboration**: Membership changes и neutral removal notices.
- **System results**: Update availability и durable failures; временный progress остаётся Toast.

Удаление из команды указывает repository и package или Maven domain, но намеренно не operator. Dedupe keys не дают
повторным проверкам переполнять inbox. Unread count виден в меню и рядом с avatar.
