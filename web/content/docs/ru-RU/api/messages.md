---
title: API центра сообщений
order: 7
category: Справочник API
description: Уведомления, счётчики, действия workflow и объявления администратора
---

# API центра сообщений

Все маршруты требуют аутентификацию. Ответы по умолчанию используют protobuf и не кешируются. API Token требует
`messages:read`; для отправки администратором также нужны `admin:notifications` и роль администратора.

## Список и очистка сообщений

- **Список**: `GET /api/messages?limit=30&cursor=...`
- **Очистить решённые**: `DELETE /api/messages`
- `limit` принимает 1–100. `cursor` — непрозрачный `next_cursor` с предыдущей страницы.
- Очистка не удаляет сообщение, пока его действие workflow имеет состояние `pending`.

### Пример декодированного ответа

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

## Число непрочитанных сообщений

- **Путь**: `GET /api/messages/unread-count`
- **Декодированный ответ**: `{"unread_count":3}`

## Прочтение и удаление

### Одно сообщение

- **Отметить прочитанным**: `POST /api/messages/:id/read`
- **Удалить**: `DELETE /api/messages/:id`
- Для чужого сообщения возвращается `404`, для незавершённого workflow — `409`.

### Все сообщения

- **Отметить все прочитанными**: `POST /api/messages/read-all`
- Ответ содержит число изменённых строк.

## Объявление администратора

- **Поиск получателей**: `GET /api/messages/admin/users?q=alice` возвращает до восьми имён.
- **Отправка**: `POST /api/messages/admin`
- Для всех аккаунтов задайте `all: true`; иначе передайте точные `recipients`. Сервер ограничивает заголовок, текст,
  важность и число получателей.

```json
{
  "recipients": ["alice", "bob"],
  "all": false,
  "severity": "warning",
  "title": "Scheduled maintenance",
  "body": "The service will restart at 02:00 UTC."
}
```

Приглашения и системные результаты создаёт соответствующий сервис. Уведомление об исключении указывает репозиторий и
пакет или домен Maven, но намеренно не раскрывает участника, выполнившего действие.
