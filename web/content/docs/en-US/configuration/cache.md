---
title: Cache Backends
order: 5
category: Configuration
description: Memory, Redis, and Valkey caches with safe credential invalidation
---

# Cache Backends

RenoP defaults to memory caches. Administrators can select Redis or Valkey in the service settings.
Backend changes take effect after a restart.

## Configuration

```yaml
cache:
  mode: memory
  address: localhost:6379
  username: ""
  password: ""
  database: 0
  tls: false
  timeout_ms: 1000
```

`mode` accepts `memory`, `redis`, or `valkey`. Use `host:port` for `address`, including brackets around IPv6 hosts.
The username is optional; password authentication and TLS are supported. TLS validates the server certificate.
The database number must be between 0 and 65535 and must exist on the selected service.
`timeout_ms` accepts 10–10000 milliseconds and applies to connection, pool wait, read, and write operations.
The pool has at most eight connections. Startup requires a reachable external cache when an external mode is configured.

## Cached Data

The backend applies to artifact metadata content, parsed Maven metadata, authentication results, database account,
session, profile and identity lookups, derived repository cache policies, upstream Docker tokens, and SPA asset payloads.
The generated HTML shell also uses the selected backend. Database storage, live sessions, locks, sockets, and workers
retain their existing ownership and persistence.

External values are encrypted and authenticated with a process-private key. Cache keys are opaque; credentials and
request paths do not appear in them. Local indexes retain the fields needed for bounded eviction and targeted
invalidation. Revocation discards these references even when remote deletion fails, and stale authentication loads
cannot refill an invalidated cache.

Existing cache capacities and logical expiration rules remain enforced. File cache payload capacity still follows
`server.file_cache_size_mb`. Each external value is limited to 2 MiB and expires within 24 hours; file, policy, and SPA
entries expire after one hour. Configure the cache server's own memory limit and eviction policy for the desired
total memory budget. RenoP does not change shared server settings.

A cache restart, eviction, missing entry, invalid ciphertext, or runtime connection failure falls back to the owning
database, storage, or generator. Connection failures bypass the shared cache for five seconds before retrying.
A RenoP restart creates a new namespace and key; old values expire naturally. No cache value is authoritative.

## Administration API

```http
GET /api/settings/cache
PUT /api/settings/cache
POST /api/settings/cache/test
```

These endpoints require a system administrator or an authorized `admin:settings` token.
GET and PUT responses never return the stored password. `password_configured` indicates whether one exists;
`restart_required: true` means changes require restarting RenoP. An empty password preserves the stored password;
`clear_password: true` removes it.

PUT accepts the fields shown above. The test endpoint accepts the same settings, preserves an omitted password,
and checks the connection without saving. Request bodies are limited to 8 KiB.
Stable errors include `cache_settings_invalid`, `cache_settings_save_failed`, and `cache_connection_failed`.

The cache account needs PING, SET, GETRANGE, and DEL access to `renop:*`, plus the authentication/database-selection
commands required by its server. Sentinel and cluster discovery are not configured by these endpoints.
