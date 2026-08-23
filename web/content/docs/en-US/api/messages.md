---
title: Message Center API
order: 7
category: API Reference
description: User notifications, unread counts, and mark-as-read endpoints
---

# Message Center API

Manage in-app notifications and workflow messages for the current user.

## 1. List Messages

- **Path**: `GET /api/messages`
- **Auth**: Required
- **Query Parameters**:
    - `limit`: Page size (default: 30, max: 100)
    - `cursor`: Pagination cursor (`next_cursor` from previous page)

### Response (JSON)

```json
{
  "messages": [
    {
      "id": "msg_01",
      "title": "CI Build Published",
      "body": "Artifact com.example:lib:1.0.0 was deployed to releases.",
      "severity": "info",
      "read": false,
      "created_at": 1740000000
    }
  ],
  "unread_count": 1,
  "next_cursor": ""
}
```

---

## 2. Unread Count

- **Path**: `GET /api/messages/unread`
- **Auth**: Required
- **Response (JSON)**: `{"unread_count": 3}`

---

## 3. Mark as Read

### Single Message

- **Path**: `POST /api/messages/:id/read`
- **Auth**: Required
- **Response**: `200 OK`, `{"success": true}`

### All Messages

- **Path**: `POST /api/messages/read-all`
- **Auth**: Required
- **Response**: `200 OK`, `{"success": true}`

---

## 4. Broadcast Announcement (Admin)

- **Path**: `POST /api/messages/broadcast`
- **Auth**: Admin
- **Request Body (JSON)**:
  ```json
  {
    "recipients": [],
    "all": true,
    "severity": "warning",
    "title": "Scheduled Maintenance",
    "body": "Server maintenance will occur this Sunday at 02:00 UTC."
  }
  ```
