---
title: Security & Permissions
order: 1
category: Security
description: Role-based access control (RBAC), repository permissions, and authentication
---

# Security & Permissions

RenoP combines role-based access control (RBAC) with granular repository-level permissions.

## 1. Role Hierarchy

| Role          | Scope                      | Description                                                           |
|:--------------|:---------------------------|:----------------------------------------------------------------------|
| **Anonymous** | PUBLIC repositories only   | Unauthenticated visitors                                              |
| **User**      | Assigned repository scopes | Standard authenticated user                                           |
| **Manager**   | Configuration and users    | Manages user accounts, tokens, and repository settings                |
| **Admin**     | Full system control        | Superuser with complete access to all repositories and system updates |

## 2. Granular Repository Permissions

| Permission String  | Description                                                          |
|:-------------------|:---------------------------------------------------------------------|
| `canview:{repo}`   | Grants read and download access to the specified repository          |
| `canupdate:{repo}` | Grants upload, deploy, and update access to the specified repository |
| `canadmin:{repo}`  | Grants administrative control over the specified repository          |
| `allview`          | Global read access across all private repositories                   |
| `allupdate`        | Global write access across all repositories                          |

## 3. Authentication Transports

1. **Cookie Session**: Managed automatically by the web UI (`renop_session`).
2. **HTTP Basic Auth**: Username + password or username + API token for standard package protocols only.
3. **Bearer Token**: `Authorization: Bearer <token>` for scope-controlled API and package automation.

Browser session secrets are cookie-only. RenoP rejects `Authorization: Session` and query-string credentials so
session and API secrets do not leak through copied URLs, referrers, or access logs.

## 4. Session & Security Policies

- **Session Expiry**: Sessions expire after 7 days of inactivity and slide forward upon active requests.
- **Password Hashing**: Passwords use salted one-way hashes; API tokens are generated from 256 random bits and stored
  only as lookup digests.
- **Brute Force Protection**: Repeated authentication failures trigger progressive IP bans.
