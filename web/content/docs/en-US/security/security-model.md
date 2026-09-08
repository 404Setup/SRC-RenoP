---
title: Security & Permissions
order: 1
category: Security
description: Authentication boundaries, repository permissions, package teams, and defense in depth
---

# Security & Permissions

RenoP authorizes each request by credential type, API-token capability, account role, repository visibility, and the
target package or domain team. No credential preserves authority after the owning account loses it.

## Account and system roles

| Role or permission                     | Effect                                                                                           |
|:---------------------------------------|:-------------------------------------------------------------------------------------------------|
| Anonymous                              | Read `PUBLIC` content and exact known paths in `HIDDEN` repositories                             |
| `base`                                 | Authenticated account without implicit repository writes                                         |
| `canview:{repo}` / `canview:*`         | Read the named repository or every repository, including private content                         |
| `canmoderate:{repo}` / `canmoderate:*` | Inspect and decide queued content for the named repository or every repository                   |
| `canupdate:{repo}` / `canupdate:*`     | Publish to the named repository or every repository, subject to package/domain policy            |
| `showing`                              | Legacy compatibility permission to discover hidden repositories in the browser catalog           |
| `allview` / `proview`                  | Legacy global private-read aliases                                                               |
| `manager` / `admin`                    | System super-administrator; users, repositories, settings, audit, updates, and all package teams |

System administrator authority is global. Package-team L0-L4 levels are separate and remain the normal authority for
package/domain collaboration. Administrator operations are recorded and do not silently create displayed team members.
Moderator permissions include private review visibility, but do not grant publication, user management, repository
configuration, or system settings access.

Administrators and moderators cannot be suspended while they hold those roles. Revoke all administrator and moderator permissions before banning an account. A suspended account must be unbanned before receiving those roles. The database checks both operations inside the account transaction, including permission changes and renames. Conflicts return `409` with `ACCOUNT_BAN_PROTECTED`; rejected bans leave existing sessions intact.

## Account and IP suspension

System administrators can suspend accounts from the users page with a reason, an optional expiry, and **Also ban recorded login IPs**. The server collects at most 64 normalized addresses from retained sessions (the 64 most recently active records) and the latest 256 successful sign-ins from the last 30 days. It also retains addresses already covered by an active IP suspension. Arbitrary addresses from the browser are not accepted. If no usable address exists, the entire operation fails with `409 ACCOUNT_BAN_IP_UNKNOWN`; turn off the option to suspend only the account.

IP restrictions, the account suspension, and session revocation commit together. Restrictions survive restarts and account renames and expire with the account ban. Startup preserves retired account tombstones without recreating credentials or memberships. Unchecking the option removes that account's IP restrictions while preserving its suspension; unbanning removes both. An address remains blocked while another account's active suspension still covers it. Omitting `ban_ip` when editing preserves the current active IP restrictions.

A blocked address receives `403` with `IP_BANNED` for HTTP requests, including sign-in, registration, authenticated requests, and public downloads. Other users sharing the address are also affected. Forwarded client addresses are accepted only from configured trusted proxies. The administrator status response exposes a count rather than the stored addresses; all suspension endpoints require system administrator authority and return private, non-cacheable metadata. API tokens also need the `admin:users` scope.

| Method | Path | Request or result |
|---|---|---|
| GET | `/api/tokens/:name/ban` | `{ban,ip_count,protected_role}` |
| PUT | `/api/tokens/:name/ban` | `{reason,expires_at,ban_ip}`; expiry may be null |
| DELETE | `/api/tokens/:name/ban` | Lift the account and its IP restrictions; `204` |

## Repository and team layers

- **Repository visibility** controls discovery and the base read boundary: `PUBLIC`, permission-gated discovery for
  `HIDDEN`, or authorized `PRIVATE`.
- **Repository permissions** grant broad read/write ability but do not create an npm/Cargo/Docker package or verify a
  Maven
  domain automatically.
- **npm/Cargo/Docker teams** use L0 read, L1 publish, L2 lifecycle/metadata, L3 member management, and L4 ownership.
- **Maven teams** attach to a verified global domain and apply in every Maven repository.
- **Private Docker images** have no implicit public L0; blob access is constrained to images the user can read.
- **Private npm packages** must be scoped and require an explicit package member or administrator.

## Credential transports

- **Browser session**: HttpOnly `renop_session` cookie, required for private account-security and Token-management UI.
- **Basic**: Username plus password or API Token, accepted only by standard package protocols.
- **Bearer API Token**: Capability and exact-target policy for API and package automation.
- **Docker Bearer**: Short-lived registry token issued only with actions allowed by the source credential and image.

`Authorization: Session`, session secrets in URLs, and query-string credentials are rejected. API Token scopes and
targets are always intersected with current account authorization.

A `403` during session restoration does not sign the browser out: an IP or authorization restriction may apply to an otherwise valid session. A `401` requires signing in again.

## Defense in depth

- Passwords and recovery codes use salted one-way verification; API Token plaintext is never persisted.
- Sessions expire after inactivity and can be revoked per device. Recovery revokes all existing sessions atomically.
- Rate limits, progressive IP bans, active-request bounds, and trusted-proxy validation protect network boundaries.
- Uploads, archive extraction, mirrors, and update packages use bounded streaming, path validation, hashes, and
  temporary
  storage. Javadoc and Cargodoc run in sandboxed viewers.
- Audit and durable messages record security-relevant outcomes without exposing operator identity where product policy
  requires neutral notifications.
