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

| Role or permission | Effect |
|:-------------------|:-------|
| Anonymous | Read `PUBLIC` content and exact known paths in `HIDDEN` repositories |
| `base` | Authenticated account without implicit repository writes |
| `canview:{repo}` / `canview:*` | Read the named repository or every repository, including private content |
| `canupdate:{repo}` / `canupdate:*` | Publish to the named repository or every repository, subject to package/domain policy |
| `showing` | Legacy compatibility permission; hidden repositories still remain unlisted in user-facing catalogs |
| `allview` / `proview` | Legacy global private-read aliases |
| `manager` / `admin` | System super-administrator; users, repositories, settings, audit, updates, and all package teams |

System administrator authority is global. Package-team L0-L4 levels are separate and remain the normal authority for
package/domain collaboration. Administrator operations are recorded and do not silently create displayed team members.

## Repository and team layers

- **Repository visibility** controls discovery and the base read boundary: `PUBLIC`, unlisted `HIDDEN`, or authorized
  `PRIVATE`.
- **Repository permissions** grant broad read/write ability but do not create a Cargo/Docker package or verify a Maven
  domain automatically.
- **Cargo/Docker teams** use L0 read, L1 publish, L2 lifecycle/metadata, L3 member management, and L4 ownership.
- **Maven teams** attach to a verified global domain and apply in every Maven repository.
- **Private Docker images** have no implicit public L0; blob access is constrained to images the user can read.

## Credential transports

- **Browser session**: HttpOnly `renop_session` cookie, required for private account-security and Token-management UI.
- **Basic**: Username plus password or API Token, accepted only by standard package protocols.
- **Bearer API Token**: Capability and exact-target policy for API and package automation.
- **Docker Bearer**: Short-lived registry token issued only with actions allowed by the source credential and image.

`Authorization: Session`, session secrets in URLs, and query-string credentials are rejected. API Token scopes and
targets are always intersected with current account authorization.

## Defense in depth

- Passwords and recovery codes use salted one-way verification; API Token plaintext is never persisted.
- Sessions expire after inactivity and can be revoked per device. Recovery revokes all existing sessions atomically.
- Rate limits, progressive IP bans, active-request bounds, and trusted-proxy validation protect network boundaries.
- Uploads, archive extraction, mirrors, and update packages use bounded streaming, path validation, hashes, and temporary
  storage. Javadoc and Cargodoc run in sandboxed viewers.
- Audit and durable messages record security-relevant outcomes without exposing operator identity where product policy
  requires neutral notifications.
