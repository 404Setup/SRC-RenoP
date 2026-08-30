---
title: Review API
order: 13
category: API Reference
description: Independent ownership-transfer requests, filtering, and single-decision processing
---

# Review API

The review API keeps ownership transfers separate from the message center. Tasks are durable, paginated, and decided
exactly once. They currently cover Docker images, npm packages, Cargo crates, Maven artifacts, and Maven publishing
domains.

## Scope and credentials

Review routes accept only an authenticated browser session. Basic credentials and Bearer API tokens cannot create,
list, decide, or cancel tasks. The account menu opens the same workflow at `/account/reviews`.

The reviewer view contains tasks for teams where the current account is T3 or T4. The requester view contains tasks
created by the current immutable account identity, including records created before a username change.

## Transfer rules

The requester must hold effective L4 ownership of the project or publishing domain, or current repository/system
administration authority. A transfer into a global team also requires membership in that team. A T3 or T4 manager of
the reviewing team or a system administrator must approve or reject the request. A requester with reviewer authority
may decide their own task.

Transfers move only the ownership binding. Package-level members are not copied or removed. Direct transfers between
two teams are not accepted: return an eligible project to personal ownership first, then submit a separate transfer.

Namespaced Docker images and scoped npm packages cannot return to personal ownership because their names reserve the
team's immutable prefix. Mirrored resources cannot be transferred.

## List tasks

GET /api/reviews returns a bounded page. `view` accepts `reviewer` or `requested`; `status` accepts `pending`,
`approved`, `rejected`, `cancelled`, or `all`. The optional comma-separated `types` filter accepts the five supported
resource types. `limit` is between 1 and 100, and `offset` is non-negative.

The response contains `tasks`, `total`, `limit`, `offset`, and the resolved `view`. A task preserves its source and
target team prefixes, requester display name, timestamps, current status, and any completed decision metadata.

## Request transfer

POST /api/reviews/super-team-transfers accepts `resource_type`, `repository`, `resource_key`, and
`target_team_prefix`. Maven publishing domains omit `repository`. Maven artifacts use a `groupId:artifactId` resource
key. An empty target requests a return to personal ownership.

Only one ownership transfer may be pending for a resource, regardless of its requested target. Creation returns
`201 Created`, the task body, and its API location.

## Decide or cancel

POST /api/reviews/{id}/decision accepts `approved` or `rejected`. Rejection requires a non-empty reason of at most 512
characters; approval ignores a supplied reason. Approval rechecks the current binding and applies the transfer in the
same database transaction as the decision.

DELETE /api/reviews/{id} lets only the requester cancel a pending task. Competing decisions use a pending-state
compare-and-set, so every later attempt receives a conflict and cannot update the resource.

## Error handling

Failures expose a stable `X-Renop-Error-Code`. A `400` response identifies malformed filters, resource identities, or
decisions. `403` indicates missing ownership, target-team membership, or reviewer authority. `404` means the task does
not exist. `409` covers duplicate pending requests, a completed task, changed ownership, or a restricted transfer.

Clients must localize the registered code and must not display the response body directly.
