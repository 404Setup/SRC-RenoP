---
title: Global and activity logs
order: 13
category: API Reference
description: Browse operational diagnostics and filter account activity
---

# Global and activity logs

## Access

Open **Global logs** from the administrator dashboard. The global endpoint requires a live system administrator; API tokens also require `admin:audit`. Repository moderator roles do not grant access. Personal activity stays scoped to the current account; an administrator can inspect another account’s activity.

The initiator is the account that requested the operation; the operator is the account or worker that executed it. Existing direct operations and historical rows use the operator as initiator. Both identity fields use the same masking and filter restrictions in personal views. Sources remain in `trigger`. Account rename and retirement cleanup cover both identities.

```http
GET /api/auth/logs?kind=system&severity=error&trigger=http&page=1&page_size=20
GET /api/auth/profile/audit-logs?action=LOGIN&from=1788739200000&until=1788825599999
GET /api/auth/users/alice/audit-logs?operator=admin&trigger=web
```

## Filters

Filters are combined with AND. Text filters use exact values; empty values remove the filter. Time controls use the browser’s local time and send Unix milliseconds. Invalid filters return `400` with `LOG_FILTER_INVALID`. The action and trigger limits are 64 bytes; account and operator limits are 255 bytes.

| Parameter | Values |
| --- | --- |
| `kind` | `audit`, `system` |
| `action` | Exact action ID, e.g. `LOGIN`, `SYSTEM_LOG`, `SYSTEM_HTTP_ERROR` |
| `operator` | Exact username; self-view also accepts `@administrator` |
| `initiator` | Exact username; self-view also accepts `@administrator` |
| `username` | Affected account, global view only |
| `trigger` | Exact source: `web`, `api`, `http`, `system`, `unknown` |
| `severity` | `info`, `warning`, `error` |
| `from`, `until` | Inclusive Unix milliseconds; either bound may be omitted |
| `page`, `page_size` | Default `1`, `20`; page size `1`–`200`; offset at most `1000000` |

## Visibility

Responses use the existing `AuditLogList` protobuf with additive `kind`, `trigger`, `initiator`, and `severity` fields. Personal and per-account activity endpoints always restrict `kind` to `audit`. Personal views mask other operators as Administrator; `operator=@administrator` selects that visible group. Guessing another operator’s username returns `400`, including unknown usernames, so filters cannot reveal a hidden administrator. The `username` filter cannot widen an account endpoint’s scope.

## System diagnostics

Process logs are captured after background services start, and HTTP server failures are recorded without returning diagnostic details to public callers. Common password, credential header, token, and URL-userinfo patterns are redacted before storage; records are capped at 4096 bytes. Existing producers derive a trigger from their authentication method; migrated historical rows use `unknown`. Unstructured process messages use keyword-based severity; structured HTTP failures use `error`.

## Retention and delivery

A single consumer writes activity and system records. Normal shutdown restores the process logger and drains queued records. The queue holds 500 records. When it is full, activity uses the existing synchronous fallback; process logs remain in the original output, increment the failure counter, and emit a queue-overflow record when capacity returns. Persistence failures write only to the original process log to prevent recursion. Activity keeps the configured `audit_log` retention and row budget; system records have a separate budget, at most 30 days and 10000 rows, using lower configured limits when present. Cleanup runs every ten minutes. Account retirement retention and permanent audit-purge rules also apply to related system records. No historical process logs are imported.
