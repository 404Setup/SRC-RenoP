---
title: API Tokens & Users
order: 3
category: API Reference
description: Fine-grained API-token lifecycle, authentication boundaries, and administrator user endpoints
---

# API Tokens & Users

API tokens are durable machine credentials owned by one account. RenoP stores only a SHA-256 lookup digest of each
256-bit random secret. The plaintext value is returned once when the token is created and cannot be recovered later.

Every request must pass two independent checks:

- The token must include the capability required by the endpoint.
- The owning account must still be allowed to perform that operation on the target resource.

Changing an account's role or package-team membership therefore takes effect without recreating its tokens.

## Manage your API tokens

Token-management endpoints require the `renop_session` HttpOnly browser cookie. API tokens, passwords,
`Authorization: Session`, and query-string credentials cannot manage token secrets.

### List assignable scopes

`GET /api/auth/profile/api-tokens/scopes`

The response is filtered by the current account. Administrator scopes are never offered to ordinary users.

```json
{
  "scopes": ["repository:read", "repository:publish", "package:metadata"],
  "target_kinds": {
    "repository:read": "repository",
    "repository:publish": "repository",
    "package:metadata": "package"
  },
  "target_limit": 128
}
```

### Create a token

`POST /api/auth/profile/api-tokens`

```json
{
  "name": "CI publishing",
  "scopes": ["repository:read", "repository:publish"],
  "targets": {
    "repository:publish": ["releases"]
  },
  "expires_at": 1798761600000
}
```

`expires_at` is an optional Unix-millisecond timestamp between five minutes and five years after creation. A null or
omitted value creates a token without a credential-level expiration. Accounts may own at most 50 API tokens.

`targets` is optional and restricts each listed scope independently. Omit a scope from `targets` to allow every target
that the owning account can currently authorize. Repository targets are exact repository names. Package targets use
`repository/package`; Maven packages append the group and artifact, such as `maven-releases/com.example/library`.
Team targets use `package/repository/package` or `domain/example.com`. Domain targets are canonical domain names.
The request may contain at most 128 target entries in total. Target restrictions never bypass repository permissions
or the package/domain team's current L0-L4 authorization.

A successful `201 Created` response is sent with `Cache-Control: no-store`:

```json
{
  "token": {
    "id": "07cdcf2e-0828-4a29-9817-cf771cc9fb0a",
    "name": "CI publishing",
    "scopes": ["repository:publish", "repository:read"],
    "targets": {"repository:publish": ["releases"]},
    "created_at": 1787731200000,
    "expires_at": 1798761600000
  },
  "secret": "rnp_pat_EXAMPLE_REDACTED_COPY_THE_REAL_VALUE_ONCE"
}
```

### List token metadata

`GET /api/auth/profile/api-tokens`

The response contains non-secret metadata and the account limit. It never contains a token secret.

### Revoke a token

`DELETE /api/auth/profile/api-tokens/{token_id}`

Successful revocation returns `204 No Content` and invalidates cached authentication immediately.

## Scope reference

| Scope | Capability |
|:------|:-----------|
| `repository:read` | Read repository catalogs, metadata, files, images, and versions |
| `repository:publish` | Publish through Maven, Cargo, Docker, files, or chunked-upload protocols |
| `repository:delete` | Delete repository files, package versions, tags, or images |
| `package:create` | Reserve new Cargo packages or Docker images after repository authorization |
| `package:metadata` | Update package descriptions and other package metadata |
| `package:lifecycle` | Archive, restore, yank, or unyank packages and versions |
| `team:manage` | View and administer Cargo, Docker, and Maven-domain teams and invitations |
| `domain:read` | Read private Maven publishing-domain configuration |
| `domain:create` | Create Maven publishing domains |
| `domain:verify` | Request or force Maven-domain ownership verification |
| `domain:delete` | Delete Maven publishing domains |
| `messages:read` | Read, mark, and remove the account's message-center entries |
| `account:read` | Read private account data and personal audit history |
| `account:write` | Update the account's public profile through the API |
| `statistics:read` | Query download statistics available to the account |
| `admin:users` | Administer user accounts and their login devices |
| `admin:repositories` | Administer repositories and rebuild repository indexes |
| `admin:settings` | Administer system settings and diagnostics |
| `admin:audit` | Read or clear administrator-visible audit and status data |
| `admin:notifications` | Compose administrator notifications |
| `admin:updates` | Check, upload, install, and restart system updates |
| `admin:statistics` | Query system-wide download statistics |

The `admin:*` scopes can be created only by an administrator and stop authorizing administrator operations as soon as
the owning account loses that role. Existing `package:manage` and `domain:manage` credentials remain compatible, but
these broad scopes are not assignable to new tokens.

## Use a token

Use a bare token as a Bearer credential for scoped API automation:

```http
Authorization: Bearer rnp_pat_REDACTED
```

Standard package clients may use the same token as the Basic password with the owning username. Basic credentials are
restricted to package protocols and cannot call management APIs.

Cargo sends the configured token as an opaque `Authorization` value; RenoP applies the same scope checks. Docker first
exchanges Basic credentials at `/v2/token`, and the issued short-lived registry token contains only the pull or push
actions allowed by both API-token scopes and package permissions.

## Compatibility endpoint

Administrator user CRUD remains under `/api/tokens`, but administrators cannot create credentials on another user's
behalf. The older `POST /api/auth/profile/token` endpoint still creates an additional non-expiring publishing token for
the signed-in account. New integrations should use the fine-grained profile endpoints.
