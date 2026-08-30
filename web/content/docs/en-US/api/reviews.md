---
title: Review API
order: 13
category: API Reference
description: Independent ownership transfers and repository publication review
---

# Review API

The review API keeps ownership transfers and moderated publications separate from the message center. Tasks are
durable, paginated, and decided exactly once. They cover Docker images, npm packages, Cargo crates, Maven artifacts,
and Maven publishing domains.

## Scope and credentials

Review routes accept only an authenticated browser session. Basic credentials and Bearer API tokens cannot create,
list, decide, or cancel tasks. The account menu opens the same workflow at `/account/reviews`.

The reviewer view contains ownership tasks for teams where the account is T3 or T4 and publication tasks for
repositories where it is a moderator. System administrators can review every task. The requester view follows the
current immutable account identity, including records created before a username change.

## Transfer rules

The requester must hold effective L4 ownership of the project or publishing domain, or current repository/system
administration authority. A transfer into a global team also requires membership in that team. A T3 or T4 manager of
the reviewing team or a system administrator must approve or reject the request. A requester with reviewer authority
may decide their own task.

Transfers move only the ownership binding. Package-level members are not copied or removed. Direct transfers between
two teams are not accepted: return an eligible project to personal ownership first, then submit a separate transfer.

Namespaced Docker images and scoped npm packages cannot return to personal ownership because their names reserve the
team's immutable prefix. Mirrored resources cannot be transferred.

## Publication rules

A Maven repository can keep review disabled, review only the first version of a new artifact, or review every version.
Enabling review disables redeployment. Local package files are committed but removed from the public index until a
repository moderator or system administrator decides the task. Mirror downloads never enter this workflow.

When detached GPG signatures are required, signature validation completes first. A successful version then enters
publication review. Files uploaded for the same version are attached to one task, including checksum, signature, and
Maven metadata companions. A five-second settling window after the latest file prevents a reviewer from deciding a
version while the client is still uploading it. An approved version is sealed against later file additions.

An npm publish is already one complete transaction. RenoP hides its tarball and retains a bounded manifest/dist-tag
payload until approval, then records the immutable version and tags together. For `new_packages`, a reservation remains
new until its first visible version is approved. Upstream packuments and tarballs never create review tasks.

## List tasks

GET /api/reviews returns a bounded page. `view` accepts `reviewer` or `requested`; `status` accepts `pending`,
`approved`, `rejected`, `cancelled`, or `all`. The optional comma-separated `types` filter accepts the five supported
resource types. `limit` is between 1 and 100, and `offset` is non-negative.

The response contains `tasks`, `total`, `limit`, `offset`, and the resolved `view`. A task preserves its source and
target team prefixes, requester display name, timestamps, current status, and any completed decision metadata.
Publication tasks also include `resource_version`, `file_count`, `total_size`, and the latest file time.

## Request transfer

POST /api/reviews/super-team-transfers accepts `resource_type`, `repository`, `resource_key`, and
`target_team_prefix`. Maven publishing domains omit `repository`. Maven artifacts use a `groupId:artifactId` resource
key. An empty target requests a return to personal ownership.

Only one ownership transfer may be pending for a resource, regardless of its requested target. Creation returns
`201 Created`, the task body, and its API location.

## Review files

GET /api/reviews/{id}/files returns at most 256 repository-relative files with a stable file identifier, size, upload
time, and critical-file marker. GET /api/reviews/{id}/files/{file_id} streams one hidden file. These routes are
available only to the requester, an assigned repository moderator, or a system administrator using a browser session.

The web review center downloads files with at most four adaptive workers and retries each failure twice. When every
file succeeds, it creates a ZIP archive in the browser using the standard repository paths. If any file still fails,
it opens the critical files individually instead of presenting an incomplete archive.

## Decide or cancel

POST /api/reviews/{id}/decision accepts `approved` or `rejected`. Ownership-transfer rejection requires a non-empty
reason of at most 512 characters. Publication rejection requires `reason_code`; supported values are
`invalid_metadata`, `quality`, `policy_violation`, `copyright`, `malware`, and `custom`. A custom reason is limited to
505 characters. Approval records the engine’s version metadata before exposing its files; rejection deletes the
hidden files. Both paths keep the durable task decision compare-and-set.

DELETE /api/reviews/{id} lets only the requester cancel a pending ownership transfer. Publication reviews cannot be
cancelled through this route. Competing decisions use a pending-state compare-and-set, so every later attempt receives
a conflict and cannot update the resource.

## Error handling

Failures expose a stable `X-Renop-Error-Code`. A `400` response identifies malformed filters, resource identities, or
decisions. `403` indicates missing ownership, target-team membership, or reviewer authority. `404` means the task or
review file does not exist. `409` covers duplicate pending requests, a completed task, changed ownership, a restricted
transfer, or a publication that is still receiving files.

Clients must localize the registered code and must not display the response body directly.
