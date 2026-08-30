---
title: Authentication API
order: 2
category: API Reference
description: Browser sessions, profiles, login methods, recovery codes, and session revocation
---

# Authentication API

Browser authentication uses the HttpOnly `renop_session` cookie. Session secrets are not returned by profile or
session-list APIs and are rejected in request headers and URLs. Private security endpoints accept only a browser
session, never a password or API token.

## Password or email login

- **Path**: `POST /api/auth/login`
- **Auth**: None.
- **Body**: protobuf `LoginRequest`; JSON field names are shown below. `name` accepts a username or private login email.

### Request

```json
{
  "name": "admin",
  "secret": "your_password"
}
```

### Session result

Success sets `renop_session` with `HttpOnly`, `SameSite=Lax`, and `Secure` when HTTPS is detected. The protobuf
`SessionDetails` body contains account permissions and routes but leaves `session_token` empty.

## Passkey and GitHub login

- **Passkey begin**: `POST /api/auth/fido/login/begin`
- **Passkey finish**: `POST /api/auth/fido/login/finish`
- **GitHub start**: `GET /api/auth/github/start`
- **GitHub callback**: `GET /api/auth/github/callback`
- **GitHub availability**: `GET /api/auth/github/status`

GitHub login appears only after an administrator configures OAuth. RenoP requests user and organization read access,
stores immutable provider IDs and current principal snapshots, and never persists the OAuth access token.

## Current account and public profiles

- **Current session**: `GET /api/auth/me`
- **Private profile**: `GET /api/auth/profile`
- **Update username or nickname**: `PUT /api/auth/profile`
- **Update password**: `PUT /api/auth/profile/password`
- **Logout**: `POST /api/auth/logout`
- **Public profile**: `GET /api/users/:username/profile`
- **Package memberships**: `GET /api/users/:username/memberships?format=cargo|docker|maven|npm`

Visible profile routes use usernames. Immutable user IDs remain internal. `HIDDEN` repository memberships are omitted;
private memberships are returned only to an authorized viewer.

## Account security

Account-security routes require the current browser session and return `Cache-Control: no-store`.

### Email and password-login policy

- **Read state**: `GET /api/auth/profile/security`
- **Set email**: `PUT /api/auth/profile/email`
- **Enable or disable password login**: `PUT /api/auth/profile/password-login`
- Password login can be disabled only while Passkey or GitHub remains linked. Enabling it requires a configured
  password.

### Recovery codes

- **Generate**: `POST /api/auth/profile/recovery-codes`
- **Reset password**: `POST /api/auth/recovery/password`
- Generation returns twelve one-time codes once. RenoP stores Argon2id verifiers, not plaintext. Recovery requires four
  distinct unused codes, consumes them atomically, revokes existing sessions, and re-enables password login.

```json
{
  "identifier": "admin@example.com",
  "codes": ["CODE-ONE", "CODE-TWO", "CODE-THREE", "CODE-FOUR"],
  "new_password": "new_secure_password"
}
```

## Login-method management

- **List Passkeys**: `GET /api/auth/profile/fido`
- **Register Passkey**: `POST /api/auth/profile/fido/register/begin` then
  `POST /api/auth/profile/fido/register/finish`
- **Delete Passkey**: `DELETE /api/auth/profile/fido/:device_id`
- **Read linked GitHub identity**: `GET /api/auth/profile/github`
- **Disconnect GitHub**: `DELETE /api/auth/profile/github`

The last working login method cannot be removed or disabled.

## Browser sessions

- **List**: `GET /api/auth/profile/sessions`
- **Revoke one**: `DELETE /api/auth/profile/sessions/:session_id`
- **Revoke every other session**: `POST /api/auth/profile/sessions/revoke-others`

Session lists expose a public ID, login method, timestamps, IP, and user agent. They never expose the cookie secret.
