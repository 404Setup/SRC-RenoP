---
title: Authentication
order: 2
category: API
---

# Authentication and sessions

Prefix: `/api/auth`

Initial account and token configuration can be provided via `tokens.yaml` (`RENOP_TOKENS`). On process startup, data is
automatically migrated and persisted into an embedded SQLite database (`renop.db` by default). Permissions are a list of
strings.

## Permissions

| Value                 | Meaning                                          |
|-----------------------|--------------------------------------------------|
| `admin` / `manager`   | Management APIs (treated as equivalent in code)  |
| `canview:*`           | Read all repositories                            |
| `canview:<repo>`      | Read one repository                              |
| `canupdate:*`         | Write all repositories                           |
| `canupdate:<repo>`    | Write one repository                             |
| `allview` / `proview` | Read PRIVATE (and similar restricted) visibility |
| `showing`             | List HIDDEN repository roots                     |

Repository visibility:

- **PUBLIC** — anonymous read
- **HIDDEN** — files readable; listing the root needs extra roles
- **PRIVATE** — needs `canview` / `allview` / `proview`, write rights on that repo, or manager

Writes (PUT/POST/DELETE artifacts) always need `canupdate` or manager.

## Login

### `POST /api/auth/login`

Body: `application/x-protobuf`, `LoginRequest`

| Field    | Type   | Constraints               |
|----------|--------|---------------------------|
| `name`   | string | 1–128 characters          |
| `secret` | string | 1–72 bytes (bcrypt limit) |

On success: `SessionDetails` (protobuf) and cookie:

- Name: `renop_session`
- HttpOnly, SameSite=Lax
- `Secure` when HTTPS (including `X-Forwarded-Proto: https` / Cloudflare visitor HTTPS)
- Max-Age ≈ 7 days

| Status | Reason                     |
|--------|----------------------------|
| 401    | Wrong username or password |
| 403    | Account expired            |
| 400    | Unreadable body            |

The session id is set only on the `renop_session` cookie. The `session_token` field in the login response is empty;
browsers use the cookie, and scripts may resend the same id as `Authorization: Session …`.

## Current user

### `GET /api/auth/me`

Returns `SessionDetails` (protobuf) for the current session. Unauthenticated → 401.

| Field           | Meaning                                                                                                                                                                                      |
|-----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_token`  | Account summary (name, created_at, permissions, …)                                                                                                                                           |
| `permissions[]` | Expanded roles (manager gets an extra `access-token:manager` entry)                                                                                                                          |
| `routes[]`      | Path permissions from canview/canupdate (`route:read` / `route:write`). Managers also get `route:write` on `*` so clients can mirror write gates without treating manager as a special case. |
| `session_token` | Set when the request used a `Session` header                                                                                                                                                 |

Write UI (browser upload panel, delete buttons) and storage PUT/POST/DELETE all require the same effective write
permission: `admin`/`manager`, `canupdate:*`, or `canupdate:<repo>`.

Refreshes the cookie if it disagrees with the current session.

## Logout

### `POST /api/auth/logout`

Invalidates the session and clears the cookie. `204 No Content`. Also 204 when there was no session.

## Profile

All of these require a logged-in user.

### `PUT /api/auth/profile/password`

Body: `application/x-protobuf`, `UpdatePasswordRequest` (also accepts JSON):

| Field          | Type   | Constraint |
|----------------|--------|------------|
| `new_password` | string | 6–72 bytes |

Response: `StatusOk` protobuf (`status: success`). Invalid length → 400.

### `POST /api/auth/profile/token`

Regenerate the upload token (one per user; old value is replaced). Response: `GenerateTokenResponse` protobuf
(`token: "<uuid>"`).

Maven / curl:

```bash
curl -u admin:UPLOAD_TOKEN -T my.jar \
  http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar
```

Either the account password or the upload token may be used as the Basic secret, depending on account setup.

### `GET /api/auth/profile/fido`

Lists registered FIDO/WebAuthn security key devices for the current user.

Response: `application/x-protobuf`, `FidoDeviceList`

| Field (each `devices[]`) | Meaning                      |
|--------------------------|------------------------------|
| `id`                     | Unique device ID             |
| `username`               | Account name                 |
| `name`                   | Custom device label          |
| `created_at`             | Creation timestamp (Unix ms) |

### `POST /api/auth/profile/fido/register/begin`

Start a FIDO registration session. Returns `session_id` and WebAuthn registration `options`.

### `POST /api/auth/profile/fido/register/finish`

Finish FIDO registration by submitting `session_id`, `name`, and browser `credential` JSON.

### `DELETE /api/auth/profile/fido/:device_id`

Delete one of your FIDO security key devices by `device_id`. Response: `StatusOk` protobuf.

### `POST /api/auth/fido/login/begin`

Begin a FIDO passwordless login flow. Optional `username`.

### `POST /api/auth/fido/login/finish`

Complete FIDO authentication. Issues browser `renop_session` Cookie and returns `SessionDetails` protobuf.

### `GET /api/auth/profile/sessions`

Lists **browser login sessions** for the current user. Basic and Bearer authentication do **not** create sessions and
never appear here. The session secret (cookie value) is **never** returned.

Response: `application/x-protobuf`, `SessionList`

| Field (each `sessions[]`) | Meaning                                                               |
|---------------------------|-----------------------------------------------------------------------|
| `public_id`               | Opaque id for revoke APIs (not the cookie secret)                     |
| `username`                | Account name                                                          |
| `ip`                      | Last seen client IP                                                   |
| `user_agent`              | Device / User-Agent string from login                                 |
| `created_at`              | Created (Unix ms)                                                     |
| `last_active`             | Last activity (Unix ms)                                               |
| `expires_at`              | Idle expiry: `last_active` + idle timeout (typically 7 days, Unix ms) |
| `current`                 | `true` when this session is making the request                        |

### `POST /api/auth/profile/sessions/revoke-others`

Revokes every browser session for the current user **except** the session making this request. Response: `StatusOk`
protobuf (`status: success`).

If the caller is authenticated with Basic/Bearer (no browser session), all of their browser sessions are revoked.

### `DELETE /api/auth/profile/sessions/:session_id`

Drop one of **your** sessions by `public_id`. Response: `StatusOk` protobuf. Missing id is a no-op. Revoking the current
session clears the cookie.

## Manager session management

Managers (`admin` / `manager`) can inspect and revoke **any** account’s browser sessions under `/api/tokens`.

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf for that user. `404` if the account does not exist. `403` if the caller is not a manager.

### `POST /api/tokens/:name/sessions/revoke-all`

Revoke all browser sessions for that user. When the manager targets **their own** account, the session making this
request is kept so they are not locked out mid-request. Response: `StatusOk` protobuf.

### `DELETE /api/tokens/:name/sessions/:session_id`

Revoke one session of that user by `public_id`. Response: `StatusOk` protobuf. Missing id is a no-op.

## How clients send credentials

| Scenario                        | Approach                                     |
|---------------------------------|----------------------------------------------|
| Browser UI                      | Cookie (set on login)                        |
| Scripts calling management APIs | `Authorization: Session …` or cookie         |
| Maven deploy                    | Basic: `username` + password or upload token |
| CI private downloads            | Basic / Bearer; PUBLIC repos need no auth    |

`Bearer name:secret` behaves like Basic (password hash or upload token).  
`Bearer <upload-token>` (no username) looks up the user via the token index.
