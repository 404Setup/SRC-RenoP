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

The reviewer view contains ownership and T2 package-creation tasks for teams where the account is T3 or T4. It contains
publication tasks for repositories where the account is a moderator, after any required team stage has completed.
System administrators can review every task. The requester view follows the current immutable account identity,
including records created before a username change.

New tasks create deduplicated message-center notices for the reviewers assigned to the current stage and for system
administrators. Advancing a T2 creation request removes team notices and creates moderator notices without telling the
requester that the package is approved. The final decision removes every remaining reviewer notice and sends the
requester a localized approved, rejected, or cancelled result. Requesters and non-administrator moderators never
receive `decided_by`; only system administrators can inspect the final decision actor.

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

For npm, both review policies hold the explicit creation request without reserving the name. A T2 team member always
starts with T3/T4 team approval. If repository creation review is enabled, that approval advances the same task to a
repository moderator; otherwise it atomically creates the package. Final approval rechecks repository permission and
live team membership before assigning the requester L4. Under `new_packages`, later versions publish normally. Under
`every_version`, RenoP also hides each tarball and retains a bounded manifest/dist-tag payload until approval, then
records the immutable version and tags together. Upstream content never creates tasks.

A Cargo publication stores and hides the crate archive without changing the sparse index or public catalog. Approval
adds the immutable version to both metadata stores before exposing the archive. Rejection removes the hidden archive.
With `new_packages`, the crate remains new until its first visible version is approved. Mirrored crates bypass review.

For Docker, T2 creation follows the same ordered team and optional repository stages as npm. Final approval rechecks
local and upstream names plus repository and live team authority before reserving the image. Under `new_packages`, later
manifests publish normally. Under `every_version`, each exact manifest remains a bounded virtual file until approval;
its reference and tag do not enter storage or catalog tables, so a new tag cannot hide an existing tag for the same
digest. Approval atomically records the manifest, blob links, tag, and task decision. Mirror imports bypass review.

## List tasks

GET /api/reviews returns a bounded page. `view` accepts `reviewer` or `requested`; `status` accepts `pending`,
`approved`, `rejected`, `cancelled`, or `all`. The optional comma-separated `types` filter accepts the five supported
resource types. `limit` is between 1 and 100, and `offset` is non-negative.

The response contains `tasks`, `total`, `limit`, `offset`, and the resolved `view`. A task preserves its source and
target team prefixes, current reviewing team, requester display name, timestamps, status, and completed decision
metadata. A non-empty `review_team_prefix` assigns the task to that team's T3/T4 members. Team approval of a T2 package
creation clears this field while preserving `target_team_prefix` and `pending` status for repository review.
Publication tasks also include `resource_version`, `file_count`, `total_size`, and the latest file time.
Explicit npm/Docker creation uses the reserved `resource_version` value `@create` and exposes its bounded JSON request
through the same file API.

## Request transfer

POST /api/reviews/super-team-transfers accepts `resource_type`, `repository`, `resource_key`, and
`target_team_prefix`. Maven publishing domains omit `repository`. Maven artifacts use a `groupId:artifactId` resource
key. An empty target requests a return to personal ownership.

Only one ownership transfer may be pending for a resource, regardless of its requested target. Creation returns
`201 Created`, the task body, and its API location.

## Review files

GET /api/reviews/{id}/files returns at most 256 repository-relative files with a stable file identifier, size, upload
time, and critical-file marker. GET /api/reviews/{id}/files/{file_id} streams one hidden file. These routes are
available only to the requester, a T3/T4 member of the currently assigned team, an assigned repository moderator after
the team stage, or a system administrator using a browser session.

The web review center downloads files with at most four adaptive workers and retries each failure twice. When every
file succeeds, it creates a ZIP archive in the browser using the standard repository paths. If any file still fails,
it opens the critical files individually instead of presenting an incomplete archive.

## Decide or cancel

POST /api/reviews/{id}/decision accepts `approved` or `rejected`. Approving a T2 package-creation task either completes
creation or returns the same task as `pending` with an empty `review_team_prefix` when repository review is required.
Ownership-transfer rejection requires a non-empty
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
