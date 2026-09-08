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

The browser sign-in page is `/account/login`. Password, Passkey, and GitHub sign-in return to the local pathname in the optional `return_to` query parameter; queries and fragments are not retained. External addresses, authentication endpoints, and values longer than 1,024 characters fall back to `/`. An expired session opens sign-in; an authenticated permission denial returns home without ending the session.

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

## Permanent account closure

`GET /api/auth/profile/retirement` returns the current account's closure plan. Both this route and
`DELETE /api/auth/profile/retirement` require a browser session. The delete request accepts JSON:

```json
{"confirmation":"alice"}
```

The confirmation must match the current username. A successful closure returns `204 No Content`; an incorrect
confirmation returns `400` with `ACCOUNT_RETIREMENT_CONFIRMATION`. Unresolved requirements return `409` with
`ACCOUNT_RETIREMENT_BLOCKED` and the current plan. The plan includes `eligible`, `protected_role`,
`super_team_owner_count`, `maven_domain_owner_count`, `package_owner_count`, and `pending_review_count`.

System administrators and repository moderators cannot close their accounts. The account must no longer hold any
global-team T4 role, own an active Maven domain, own a non-deprecated package at L4, or have a pending review request.
Transfer ownership, close publishing domains, or permanently deprecate packages before retrying.

Closure permanently locks the account and username, removes all team memberships, releases GitHub login, and removes
Passkeys, sessions, API tokens, the profile photo, recovery codes, and messages. Login returns `ACCOUNT_DELETED`.
The private email remains reserved for 14 days; activity remains for 30 days. Bounded scheduled cleanup releases
expired holds. Published packages remain downloadable.

The released GitHub identity can immediately be linked to another active account. This does not release the retired
username, shorten the email hold, or restore ownership of retired resources.

System administrators can read deadlines with `GET /api/tokens/:name/retention`, release email early with
`DELETE /api/tokens/:name/retention/email`, and clear activity early with
`DELETE /api/tokens/:name/retention/audit`. Deadline and completion fields use Unix milliseconds:
`deleted_at`, `email_release_at`, `email_released_at`, `audit_purge_at`, and `audit_purged_at`.
Administrator `DELETE /api/tokens/:name` follows the same permanent closure requirements.
