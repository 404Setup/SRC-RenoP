---
title: Ticket API
order: 13
category: API Reference
description: Tickets combine feedback, suggestions, reports, ownership transfers, and publication approval in one durable workflow. The ticket center replaces the former review center; existing workflow IDs and pending publications are preserved.
---

# Ticket API

Tickets combine feedback, suggestions, reports, ownership transfers, and publication approval in one durable workflow. The ticket center replaces the former review center; existing workflow IDs and pending publications are preserved.

## Scope and credentials

Every route requires an active browser `renop_session` cookie. Basic credentials, Bearer API tokens, and session tokens without that cookie are rejected. Open `/account/tickets`; old `/account/reviews` links redirect there.

Repository moderators can inspect every ticket status in their scope, including the team stage. System administrators can inspect all scopes. T3/T4 team members handle only their assigned ownership and creation workflows. Team approval still precedes repository approval. Requester history follows immutable account identity.

Handling staff can see peer staff identities inside authorized tickets. Requesters never receive `assignee`, `escalated_by`, or `decided_by`. A reported account cannot read or handle its report even if it is an administrator. The reported target never receives reporter identity or access to the ticket.

Pending notices follow the current approval stage. Final outcomes notify the requester through messages and, when enabled, email in their account language. A reported target receives a separate notice only when a report completes with `upheld`; it contains the resource and outcome, without the ticket ID, reporter, staff, or private text. Dismissal and withdrawal do not notify the target. Existing email scene identifiers remain compatible.

Report audit events omit account, operator, session, and IP identity; staff attribution remains inside authorized tickets.

## Submit feedback, suggestions, or reports

POST /api/tickets accepts `kind` (`feedback`, `suggestion`, or `report`), `title` (1–160 characters), `body` (1–8000 characters), and optional `repository`. The JSON body is limited to 48 KiB. Support requests allow at most 16 pending and 24 new requests per account per 24 hours; all workflows share a global 4096 pending limit.

A report also requires `target`: `format`, `repository`, `name`, and optional `version`. Formats are `user`, `superteam`, `maven-domain`, `maven`, `cargo`, `npm`, and `docker`. Global resources omit the repository. Package reports must match the repository format and current read access. Report another user's account, visible package/version, publishing domain, or team; reporting your own resource, hidden resources, and duplicate pending reports is rejected.

Creation returns `201`, the ticket, and `Location`. A report records the target owners by immutable account IDs. Recording `upheld` does not automatically ban an account or lock a package: apply the appropriate existing moderation action before recording a penalty outcome.

```json
{"kind":"report","title":"Package report","body":"Please investigate this version.","target":{"format":"npm","repository":"npm","name":"@platform/tool","version":"1.0.0"}}
```

## Claim, escalate, and resolve

POST /api/tickets/{id}/action accepts `action`: `claim`, `release`, `escalate`, `process`, `complete`, or `close`. Staff must claim a ticket before processing or deciding it. The claim is atomic; other staff retain read access but cannot process it. A system administrator may claim with `force: true` to take over a moderator's ticket, but cannot displace another administrator who has not released it.

Escalation releases the ticket and restricts the next claim to system administrators. An administrator may escalate to a different administrator. Each ticket permits three escalations. After the third, the next assignee must finish: release, further escalation, and forced takeover are disabled. Team approval releases assignment for the next repository stage.

For support tickets, `process` records a handled state without final notification; `complete` finishes it and `close` closes it. These actions require a non-empty `response` of at most 4096 characters. Feedback/suggestions use `outcome: resolved`; reports use `upheld` or `dismissed`; closing records `closed`. Action JSON is limited to 24 KiB. Publication and ownership decisions use the decision route below after claiming.

GET /api/tickets/{id} returns the authorized detail and an `actions` array derived from current permissions and assignment. Use these actions to present controls. Assignment conflicts return `409` with `ticket_claim_required`, `ticket_occupied`, or `ticket_escalation_limit`.

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

GET /api/tickets returns a bounded page. `view` accepts `reviewer` or `requested`; `status` accepts `unprocessed` (default), `in_progress`, `processed`, `closed`, `completed`, or `all`. `limit` is 1–100 and `offset` is non-negative. The comma-separated `types` filter accepts workflow resource types plus `support`, `user`, `superteam`, `maven-domain`, `maven`, `cargo`, `npm`, and `docker`.

`ticket_status` is the shared lifecycle. The existing `status` field retains the workflow result (`pending`, `approved`, `rejected`, or `cancelled`). Existing records acquire ticket state when first claimed.

The response contains `tasks`, `total`, `limit`, `offset`, and the resolved `view`. A task preserves its source and
target team prefixes, current reviewing team, requester display name, timestamps, status, and completed decision
metadata. A non-empty `review_team_prefix` assigns the task to that team's T3/T4 members. Team approval of a T2 package
creation clears this field while preserving `target_team_prefix` and `pending` status for repository review.
Publication tasks also include `resource_version`, `file_count`, `total_size`, and the latest file time.
Explicit npm/Docker creation uses the reserved `resource_version` value `@create` and exposes its bounded JSON request
through the same file API.

## Request transfer

POST /api/tickets/super-team-transfers accepts `resource_type`, `repository`, `resource_key`, and
`target_team_prefix`. Maven publishing domains omit `repository`. Maven artifacts use a `groupId:artifactId` resource
key. An empty target requests a return to personal ownership.

Only one ownership transfer may be pending for a resource, regardless of its requested target. Creation returns
`201 Created`, the task body, and its API location.

## Review files

GET /api/tickets/{id}/files returns at most 256 repository-relative files with a stable file identifier, size, upload
time, and critical-file marker. GET /api/tickets/{id}/files/{file_id} streams one hidden file. These routes are
available only to the requester, a T3/T4 member of the currently assigned team, a repository moderator in scope, or a system administrator using a browser session.

The web review center downloads files with at most four adaptive workers and retries each failure twice. When every
file succeeds, it creates a ZIP archive in the browser using the standard repository paths. If any file still fails,
it opens the critical files individually instead of presenting an incomplete archive.

## Decide or cancel

The current actor must hold the ticket claim for this stage. POST /api/tickets/{id}/decision accepts `approved` or `rejected`. Approving a T2 package-creation task either completes
creation or returns the same task as `pending` with an empty `review_team_prefix` when repository review is required.
Ownership-transfer rejection requires a non-empty
reason of at most 512 characters. Publication rejection requires `reason_code`; supported values are
`invalid_metadata`, `quality`, `policy_violation`, `copyright`, `malware`, and `custom`. A custom reason is limited to
505 characters. Approval records the engine’s version metadata before exposing its files; rejection deletes the
hidden files. Both paths keep the durable task decision compare-and-set.

DELETE /api/tickets/{id} lets the requester withdraw a pending support, ownership-transfer, or Maven-restoration request. It closes the ticket and prevents later decisions. Publication tasks cannot be cancelled through this route. Concurrent final decisions cannot modify the resource twice.

## Error handling

Failures expose a stable `X-Renop-Error-Code`. A `400` response identifies malformed filters, resource identities, or
decisions. `403` indicates missing ownership, target-team membership, or reviewer authority. `404` means the task or
review file does not exist. `409` covers duplicate pending requests, a completed task, changed ownership, a restricted
transfer, or a publication that is still receiving files.

Clients must localize the registered code and must not display the response body directly.

## Restore a reclaimed Maven artifact

`POST /api/tickets/maven-restorations` requires the current publishing-domain L4 owner's active `renop_session` cookie:

```json
{"resource_type":"maven_artifact","repository":"releases","resource_key":"com.example:demo"}
```

It returns a `maven_restore` task with status `pending` and HTTP `201`; an equivalent pending request returns `409`.
The repository's moderators and system administrators see the task. Existing decision and cancellation routes apply.
Approval atomically rechecks the domain claim, live ownership, and independent locks before restoring publication
and the current domain-team binding. A changed claim cancels the stale request. Rejection or cancellation preserves
the artifact's restriction and downloads. Pending requests are limited to 64 per account and 4096 overall.
These tasks have no downloadable review bundle.
