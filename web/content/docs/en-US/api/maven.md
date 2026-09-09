---
title: Maven Registry API
order: 4
category: API Reference
description: Verified publishing domains, domain teams, artifact catalogs, and Maven client access
---

# Maven Registry API

RenoP Maven repositories use verified reverse-domain namespaces. A publisher reserves a domain once from the signed-in
account menu before uploading an artifact to any Maven repository. Standard Maven 2 paths, metadata, detached
signatures, and checksum companions remain compatible with Maven and Gradle clients.

## Domain verification

Create a domain with `POST /api/maven/domains`. RenoP returns a high-entropy verification code and one fixed proof
target:

- DNS namespaces use a TXT record at the registered root. RenoP reads every TXT value and accepts an exact match.
- `io.github.<account>` namespaces use the Bio of a public GitHub user or the Description of a public GitHub
  organization.
- `io.gitlab.<account>` namespaces use the Bio of a public GitLab user or the Description of a public GitLab group.

Start an external check with `POST /api/maven/domains/:domain/verify`. Checks are limited to one attempt every five
seconds for each domain. A system administrator can use `/verify/force`; this bypass is recorded in the audit log.

A verified domain and its team are global to the RenoP instance. The same domain can publish to every Maven repository
without another verification, domain reservation, or invitation cycle.

A recent GitLab.com connection can also verify a new namespace automatically: the account must own the personal namespace or a top-level group, and its proof must be less than one hour old. Public membership, subgroup ownership, and self-hosted instances are insufficient. See [Third-party Login](../security/oauth-login.md).

## Domain permissions

Maven teams are attached globally to domains rather than repositories or individual artifacts:

- L0: read public content
- L1: publish artifacts
- L2: manage versions and descriptions
- L3: invite and manage team members
- L4: own and transfer the domain

The member API accepts between one and twenty usernames in one request. Non-administrator additions create
message-center invitations. Exactly one L4 owner is retained during transfers; an owner cannot leave before transferring
ownership.

## Artifact catalog

Use `GET /api/maven/repositories/:repo/domains` to list domains containing artifacts in one repository, and
`GET /api/maven/repositories/:repo/packages` to page or search its catalog.
`GET /api/maven/repositories/:repo/package?group=...&artifact=...` returns artifact details and versions. L2 members can
update descriptions and delete complete versions through the corresponding JSON endpoints.

The detail response summarizes indexed primary files, sizes, modification times, available checksums, and
detached-signature coverage. It returns at most 64 primary files per version. When the latest indexed POM is no larger
than 2 MiB, RenoP also streams and parses its project, organization, license, developer, source-control, issue-tracker,
parent, and direct-dependency metadata. Direct dependencies are limited to 128 entries; companion checksum and signature
files are not counted as primary files.

An L2-L4 domain member or administrator can maintain a separate package-level Markdown README through the artifact
update endpoint. The README is limited to 512 KiB, returned only by the detail endpoint, and rendered through the
shared element and URL allowlist. The short POM or catalog description remains a distinct field.

Legacy Maven repositories are indexed into the domain catalog during upgrade. Imported domains are verified but receive
no automatic team members; an administrator must explicitly assign access before new publication. Configured Maven
mirrors continue to resolve missing artifacts.

## Layouts and file repositories

Maven repositories default to the domain catalog UI. An administrator can switch a Maven repository to the classic
file-tree layout and switch back later. The classic layout changes presentation only: arbitrary paths are rejected, and
publication still requires a verified domain and a valid Maven artifact or metadata path.

The separate `files` repository format is intended for unstructured content. It supports direct upload, replacement,
deletion, S3 storage, and mirrors. It deliberately does not generate checksums, generate POM files, or perform OpenPGP
signature processing.

## Maven client access

Artifact reads and publications use `/{repo}/{maven-path}`. Authenticate Maven or Gradle with an account password or
an API token carrying `repository:read` and/or `repository:publish`. Repository visibility controls reads, while
verified domain membership and the owning account's L0-L4 domain level control mutation. The complete endpoint and
schema list is available in `web/assets/openapi.yaml`.

## Resource locks

Administrators and moderators for this repository manage artifact or version locks using an active browser session cookie:

- `PUT /api/maven/repositories/{repo_name}/package/locks?group={group}&artifact={artifact}`
- `DELETE /api/maven/repositories/{repo_name}/package/locks?group={group}&artifact={artifact}`

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

Omit `version` or use an empty string to target the artifact. DELETE accepts `{"version":"1.2.3"}` and removes only that manual lock; system locks remain independent. API tokens cannot manage locks. The UI localizes public reasons: `hold`, `prohibited`, `expired`, `trojan`, `abuse`, `dmca`, `reup`, `squatting`, and `quality`.

Both modes freeze writes. `write` preserves stored downloads; `read` also restricts metadata to administrators, repository moderators, domain owners/collaborators (including L0), and members of either bound global team. File bytes, POM downloads, signatures, and checksums are unavailable to every viewer. Permitted viewers can inspect catalog details and parsed `maven-metadata.xml`; other viewers cannot discover the restricted artifact or version through catalogs, search, directory listings, version APIs, or statistics. Shared XML metadata omits hidden versions and adjusts latest/release values without changing the stored file.

Locks cover arbitrary companions and timestamped SNAPSHOT files. Frozen bytes neither expire nor refill from mirrors. Shared metadata cannot be overwritten while a version is locked; whole-artifact deprecation and repository reconfiguration/deletion are also blocked. Details expose public `locks`, `moderator`, `member`, and `version_locked` state. Denied mutations return `423` with `X-Renop-Error-Code: resource_locked`; denied reads return `404`.

Version deletion validates every coordinate segment before changing storage; path separators and dot-directory aliases are rejected.

Global-team locks are inherited by bound artifacts and publishing domains. Domain responses include public `locks`. A locked domain freezes verification, membership, ownership transfers, closure, and new publication, including uncatalogued files and metadata. Read locks preserve metadata for existing collaborators and authorized moderators while blocking all file bytes. An independently verified child domain retains its own authority. Team membership and domain permissions are retained when locked and restored after all applicable restrictions are removed.
