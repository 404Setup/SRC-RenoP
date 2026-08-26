---
title: Download Statistics API
order: 14
category: API Reference
description: Bounded download accounting, hierarchical queries, repository controls, and API-token requirements
---

# Download Statistics API

RenoP aggregates successful package downloads without storing one database row per request. Counters include download
count, logical bytes, and the latest update time. User attribution is bound to the account’s immutable identity, so a
username change does not split its history.

Maven, Cargo, and Docker repositories count by default. The unstructured `files` engine opts in. Checksum, detached
signature, Maven metadata, and Javadoc companion requests are excluded. `HEAD`, `304`, failed requests, and noninitial
range segments are not counted. Docker records one pull when a manifest is returned rather than counting every blob.

## Account queries

`GET /api/statistics` returns statistics for the API-token owner. `GET /api/statistics/users/:username` returns the same
account boundary; querying a different account requires a system administrator token.

Both endpoints require a Bearer API token with `statistics:read`. Browser session cookies and Basic credentials are
rejected. Pending in-memory counters are flushed before a query, so a successful response includes downloads already
accepted by the current server process.

## System queries

`GET /api/statistics/system` requires a system administrator account and the `admin:statistics` scope. It can group by
`user`, `repository`, `namespace`, `package`, or `version`. Account endpoints support every grouping except `user`.

The optional exact filters are `username` (system only), `repository`, `format`, `namespace`, `package`, and `version`.
Pagination uses `limit` from 1 to 100 and a zero-based `offset` up to 1,000,000. Every page also returns aggregate
`count` and `bytes` for the complete filtered result, plus the total number of grouped records.

## Repository controls

Administrators read effective switches with `GET /api/settings/repositories/download-statistics` and update one switch
with `PUT /api/settings/repositories/:name/download-statistics`. The JSON body is `{"enabled": true}` or
`{"enabled": false}`.

`DELETE /api/settings/repositories/:name/download-statistics` permanently clears both persisted and pending counters.
For Docker repositories it also resets the compatibility pull count displayed on image pages. Removing a repository
clears its statistics automatically.

The complete response schemas and parameter limits are defined in `web/assets/openapi.yaml`.
