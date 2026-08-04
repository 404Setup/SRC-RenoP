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

Single account as **protobuf** `AccessTokenDto` (`application/x-protobuf`). Names are case-insensitive (stored
lowercased). Missing → 404.

## `PUT /api/tokens/:name`

Create or update. Body: `application/x-protobuf`, `CreateAccessTokenRequest` (also accepts JSON).

| Field         | Meaning                                                                                  |
|---------------|------------------------------------------------------------------------------------------|
| `is_create`   | `true` and name already exists → 409                                                     |
| `secret`      | On create, omit to generate a UUID password; on update, omit to leave password unchanged |
| `new_name`    | Rename; target conflict → 409                                                            |
| `permissions` | Replaces the permission list only when provided                                          |

Response: `application/x-protobuf`, `CreateAccessTokenResponse`

```protobuf
message CreateAccessTokenResponse {
  AccessTokenDto access_token = 1;
  string secret = 2; // present only when generated or supplied this request
}
```

Save `secret` immediately after create — plaintext passwords are not recoverable later.

## `DELETE /api/tokens/:name`

Delete account. `204`. Missing → 404.

## Browser sessions and FIDO devices (manager)

Managers can list and revoke **browser login sessions** and **FIDO security key devices** for any account. Basic/Bearer
credentials are not sessions. Session secrets are never returned.

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf. `404` if the account does not exist.

### `POST /api/tokens/:name/sessions/revoke-all`

Revoke all browser sessions for that user. When the manager targets their **own** account, the session making this
request is kept. Response: `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Revoke one session by `public_id`. Response: `StatusOk` protobuf. Missing id is a no-op.

### `GET /api/auth/users/:username/fido`

Manager endpoint to list registered FIDO devices for the specified user. Response: `FidoDeviceList` protobuf.

### `DELETE /api/auth/users/:username/fido/:device_id`

Manager endpoint to delete a registered FIDO device for the specified user. Response: `StatusOk` protobuf.

## `POST /api/tokens/:name/token`

Admin re-issues the upload token for a user (replaces the old one). Response: `GenerateTokenResponse` protobuf
(`token: "<uuid>"`).

Same idea as `/api/auth/profile/token`, but for another user.
