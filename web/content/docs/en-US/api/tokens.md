---
title: Tokens & Users API
order: 3
category: API Reference
description: Personal Access Tokens (PAT), upload tokens, and user management endpoints
---

# Tokens & Users API

## 1. List Tokens

- **Path**: `GET /api/tokens`
- **Auth**: Required (Regular users see own tokens; Managers/Admins see all)

### Response (JSON)

```json
{
  "tokens": [
    {
      "id": "tok_123456",
      "name": "CI-Deploy-Token",
      "user": "ci_bot",
      "token_type": "upload",
      "scopes": ["canupdate:releases"],
      "created_at": 1740000000,
      "expires_at": 1771536000
    }
  ]
}
```

---

## 2. Create Token

- **Path**: `POST /api/tokens`
- **Auth**: Required

### Request Body (JSON)

```json
{
  "name": "Local-Maven-Token",
  "token_type": "pat",
  "scopes": ["canview:releases", "canupdate:snapshots"],
  "expires_in_days": 90
}
```

### Response

- **Status**: `201 Created`
- **Body (JSON)**: Returns the raw token string once:

```json
{
  "id": "tok_123456",
  "token": "renop_pat_abcdef1234567890...",
  "name": "Local-Maven-Token"
}
```

---

## 3. Revoke Token

- **Path**: `DELETE /api/tokens/:id`
- **Auth**: Token owner or Admin

### Response

- **Status**: `204 No Content`

---

## 4. User Accounts Management (Manager / Admin)

### List Users

- **Path**: `GET /api/auth/users`
- **Auth**: Manager or Admin

### Create User

- **Path**: `POST /api/auth/users`
- **Auth**: Manager or Admin
- **Body**:
  ```json
  {
    "username": "developer1",
    "password": "InitialPassword123!",
    "role": "user",
    "permissions": ["canview:releases", "canupdate:snapshots"]
  }
  ```

### Delete User

- **Path**: `DELETE /api/auth/users/:username`
- **Auth**: Admin
