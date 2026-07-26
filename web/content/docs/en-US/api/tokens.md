---
title: Tokens
order: 3
category: API
---

# Users and access tokens

Prefix: `/api/tokens`

Every endpoint requires **manager / admin**. Regular users change their own password or upload token via
`/api/auth/profile/*`.

A “token” here is an account record: username, password hash, permissions, optional upload token. Persisted in
`tokens.yaml`.

## `GET /api/tokens`

List all accounts. Response: `application/x-protobuf`, `AccessTokenList`.

Shape (JSON illustration):

```json
{
  "tokens": [
    {
      "identifier": {"type": "PERSISTENT", "value": 1},
      "name": "admin",
      "created_at": "2026-01-01T00:00:00Z",
      "description": "…",
      "expires_at": null,
      "tokens": ["<upload-token-if-any>"],
      "permissions": ["manager", "canview:*", "canupdate:*"]
    }
  ]
}
```

Password hashes are never returned. The `tokens` array holds plaintext upload tokens when present. Forbidden → 403.

## `GET /api/tokens/:name`

Single account as **JSON**. Names are case-insensitive (stored lowercased). Missing → 404.

## `PUT /api/tokens/:name`

Create or update.

```json
{
  "permissions": ["manager", "canview:releases", "canupdate:releases"],
  "secret": "optional-password",
  "new_name": "optional-rename",
  "is_create": true
}
```

| Field         | Meaning                                                                                  |
|---------------|------------------------------------------------------------------------------------------|
| `is_create`   | `true` and name already exists → 409                                                     |
| `secret`      | On create, omit to generate a UUID password; on update, omit to leave password unchanged |
| `new_name`    | Rename; target conflict → 409                                                            |
| `permissions` | Replaces the permission list only when provided                                          |

Response:

```json
{
  "access_token": {"…": "AccessTokenDto"},
  "secret": "present only when generated or supplied this request"
}
```

Save `secret` immediately after create — plaintext passwords are not recoverable later.

## `DELETE /api/tokens/:name`

Delete account. `204`. Missing → 404.

## Browser sessions (manager)

Managers can list and revoke **browser login sessions** for any account. Basic/Bearer credentials are not sessions.
Session secrets are never returned. See also self-service endpoints under `/api/auth/profile/sessions`
in [Authentication](./authentication.md).

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf. `404` if the account does not exist.

### `POST /api/tokens/:name/sessions/revoke-all`

Revoke all browser sessions for that user. When the manager targets their **own** account, the session making this
request is kept. Response: `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Revoke one session by `public_id`. Response: `StatusOk` protobuf. Missing id is a no-op.

## `POST /api/tokens/:name/token`

Admin re-issues the upload token for a user (replaces the old one).

```json
{"token": "<uuid>"}
```

Same idea as `/api/auth/profile/token`, but for another user.
