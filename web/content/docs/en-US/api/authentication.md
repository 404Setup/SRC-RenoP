---
title: Authentication API
order: 2
category: API Reference
description: Login, logout, session discovery, and password modification endpoints
---

# Authentication API

## 1. Login

- **Path**: `POST /api/auth/login`
- **Auth**: None

### Request Body (JSON)

```json
{
  "username": "admin",
  "password": "your_password"
}
```

### Response

- **Status**: `200 OK`
- **Header**: `Set-Cookie: renop_session=<session_id>; Path=/; HttpOnly; SameSite=Lax`
- **Body (JSON)**:

```json
{
  "success": true,
  "session_id": "abc123xyz...",
  "user": {
    "username": "admin",
    "role": "admin",
    "permissions": ["allview", "allupdate"]
  }
}
```

---

## 2. Logout

- **Path**: `POST /api/auth/logout`
- **Auth**: Required

### Response

- **Status**: `200 OK`
- **Body**: `{"success": true}`

---

## 3. Current User Profile

- **Path**: `GET /api/auth/me`
- **Auth**: Required

### Response (JSON)

```json
{
  "username": "admin",
  "role": "admin",
  "permissions": ["allview", "allupdate"],
  "email": "admin@example.com"
}
```

---

## 4. Change Password

- **Path**: `POST /api/auth/change-password`
- **Auth**: Required

### Request Body (JSON)

```json
{
  "old_password": "current_password",
  "new_password": "new_secure_password"
}
```

### Response

- **Status**: `200 OK`
- **Body**: `{"success": true}`

---

## 5. Audit Logs

- **Path**: `GET /api/auth/logs`
- **Auth**: Manager or Admin
- **Query Parameters**:
    - `limit`: Result count (default: 50, max: 200)
    - `offset`: Pagination offset

### Response (JSON)

```json
{
  "total": 120,
  "logs": [
    {
      "id": 1,
      "username": "admin",
      "action": "LOGIN_SUCCESS",
      "ip": "192.168.1.100",
      "timestamp": 1740000000
    }
  ]
}
```
