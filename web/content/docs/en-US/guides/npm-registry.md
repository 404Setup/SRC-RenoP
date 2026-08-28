---
title: npm Registry
order: 4
category: Guides
description: Reserving packages and using npm, pnpm, Yarn, or Bun with RenoP
---

# npm Registry Guide

Create a repository with format `npm`, then reserve each package from the repository page before publishing. RenoP
does not let a client create a package name implicitly. The examples use repository `javascript` and package
`@example/library`.

## Configure a client

Create an expiring API Token with repository read and publish scopes. Add package lifecycle or team management only
when the automation needs those operations. For a dedicated registry, place this in the project or user `.npmrc`:

```ini
registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

To route only one scope through RenoP, keep the default registry and configure the scope separately:

```ini
@example:registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

Use HTTPS outside a trusted local development network. API Tokens are preferred for automation; account passwords
remain limited to standard package-protocol authentication.

## Prepare and publish a package

The reserved name and the `name` field in `package.json` must match exactly. Versions use semantic versioning and are
immutable after a successful publication.

```json
{
  "name": "@example/library",
  "version": "1.0.0",
  "description": "Example library",
  "publishConfig": {
    "registry": "https://packages.example.com/javascript/"
  }
}
```

Publish and consume the package with any compatible client:

```bash
npm publish
npm install @example/library
pnpm add @example/library
yarn add @example/library
bun add @example/library
```

RenoP validates the bounded gzip tarball, requires `package/package.json` to match the request, computes npm-compatible
SHA-1 and SHA-512 integrity values, and commits the archive only after validation succeeds.

## Visibility and package teams

Public packages follow repository visibility. Private packages must be scoped and require explicit package membership
or administrator access. L0 reads, L1 publishes versions, L2 manages versions and metadata, L3 manages the package
team, and L4 owns the package. Removing or demoting members cannot leave a package without an L4 owner.

Use the package page to invite existing RenoP accounts. Invitations are durable message-center actions. A mirrored
package has no local team, is labeled with its upstream origin, and remains pull-only.

## Dist-tags, deprecation, and unpublish

Standard npm commands manage distribution tags and deprecation metadata:

```bash
npm dist-tag add @example/library@1.0.0 stable
npm deprecate @example/library@1.0.0 "Use version 2"
npm unpublish @example/library@1.0.0
```

Unpublishing tombstones the version and removes its tarball, but the version number cannot be reused. Deleting a
package tombstones every version and keeps the package name reserved.

## Upstream mirrors

An npm repository can proxy an ordered upstream registry. Exact package names and `@scope/*` rules constrain mirror
access. Refreshed packuments are bounded, concurrent refreshes are coalesced, local tarball URLs replace upstream URLs,
and upstream versions that disappear are removed from the local catalog. Mirror-discovered packages cannot receive local pushes.
