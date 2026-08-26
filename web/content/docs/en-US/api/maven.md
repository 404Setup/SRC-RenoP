---
title: Maven Registry API
order: 4
category: API Reference
description: Verified publishing domains, domain teams, artifact catalogs, and Maven client access
---

# Maven Registry API

RenoP Maven repositories use verified reverse-domain namespaces. A publisher reserves a domain once from the signed-in account menu before uploading an artifact to any Maven repository. Standard Maven 2 paths, metadata, detached signatures, and checksum companions remain compatible with Maven and Gradle clients.

## Domain verification

Create a domain with `POST /api/maven/domains`. RenoP returns a high-entropy verification code and one fixed proof target:

- DNS namespaces use a TXT record at the registered root. RenoP reads every TXT value and accepts an exact match.
- `io.github.<account>` namespaces use the Bio of a public GitHub user or the Description of a public GitHub organization.
- `io.gitlab.<account>` namespaces use the Bio of a public GitLab user or the Description of a public GitLab group.

Start an external check with `POST /api/maven/domains/:domain/verify`. Checks are limited to one attempt every five seconds for each domain. A system administrator can use `/verify/force`; this bypass is recorded in the audit log.

A verified domain and its team are global to the RenoP instance. The same domain can publish to every Maven repository without another verification, domain reservation, or invitation cycle.

## Domain permissions

Maven teams are attached globally to domains rather than repositories or individual artifacts:

- L0: read public content
- L1: publish artifacts
- L2: manage versions and descriptions
- L3: invite and manage team members
- L4: own and transfer the domain

The member API accepts between one and twenty usernames in one request. Non-administrator additions create message-center invitations. Exactly one L4 owner is retained during transfers; an owner cannot leave before transferring ownership.

## Artifact catalog

Use `GET /api/maven/repositories/:repo/domains` to list domains containing artifacts in one repository, and `GET /api/maven/repositories/:repo/packages` to page or search its catalog. `GET /api/maven/repositories/:repo/package?group=...&artifact=...` returns artifact details and versions. L2 members can update descriptions and delete complete versions through the corresponding JSON endpoints.

The detail response summarizes indexed primary files, sizes, modification times, available checksums, and detached-signature coverage. It returns at most 64 primary files per version. When the latest indexed POM is no larger than 2 MiB, RenoP also streams and parses its project, organization, license, developer, source-control, issue-tracker, parent, and direct-dependency metadata. Direct dependencies are limited to 128 entries; companion checksum and signature files are not counted as primary files.

Legacy Maven repositories are indexed into the domain catalog during upgrade. Imported domains are verified but receive no automatic team members; an administrator must explicitly assign access before new publication. Configured Maven mirrors continue to resolve missing artifacts.

## Layouts and file repositories

Maven repositories default to the domain catalog UI. An administrator can switch a Maven repository to the classic file-tree layout and switch back later. The classic layout changes presentation only: arbitrary paths are rejected, and publication still requires a verified domain and a valid Maven artifact or metadata path.

The separate `files` repository format is intended for unstructured content. It supports direct upload, replacement, deletion, S3 storage, and mirrors. It deliberately does not generate checksums, generate POM files, or perform OpenPGP signature processing.

## Maven client access

Artifact reads and publications use `/{repo}/{maven-path}`. Authenticate Maven or Gradle with an account password or
an API token carrying `repository:read` and/or `repository:publish`. Repository visibility controls reads, while
verified domain membership and the owning account's L0-L4 domain level control mutation. The complete endpoint and
schema list is available in `web/assets/openapi.yaml`.
