---
title: npm Registry API
order: 7
category: API Reference
description: npm package metadata, publication, tarballs, dist-tags, teams, and management endpoints
---

# npm Registry API

Each repository with format `npm` exposes an npm-compatible JSON registry below `/{repo}/`. Package names must be
reserved through the management API or web interface before the first publication.

## Registry discovery and identity

- **Availability**: `GET /{repo}/-/ping`
- **Current account**: `GET /{repo}/-/whoami`
- **Search**: `GET /{repo}/-/v1/search?text={query}&size={limit}&from={offset}`

Registry failures use JSON with stable `error` and `reason` fields:

```json
{
  "error": "not_found",
  "reason": "npm package was not found"
}
```

## Package metadata and tarballs

- **Full or abbreviated packument**: `GET /{repo}/{package}`
- **Tarball**: `GET /{repo}/{package}/-/{name}-{version}.tgz`
- **Publish or edit metadata**: `PUT /{repo}/{package}`

Scoped package names may be encoded as one path parameter, such as `%40example%2Flibrary`. Packument responses support
ETag and Last-Modified validators. Clients requesting `application/vnd.npm.install-v1+json` receive bounded abbreviated
metadata. Private responses disable shared caching.

A publication document may contain one semantic version and one base64 tarball attachment. The JSON body is limited to
96 MiB, the compressed tarball to 64 MiB, unpacked content to 512 MiB, file entries to 100,000, and `package.json` to
2 MiB. A package retains at most 5,000 version records and 4 MiB of aggregate stored version metadata. The server
streams decoded archive bytes into staging and never publishes a partially validated tarball.

## Dist-tags and lifecycle

- **List tags**: `GET /{repo}/-/package/{package}/dist-tags`
- **Set a tag**: `PUT /{repo}/-/package/{package}/dist-tags/{tag}`
- **Delete a tag**: `DELETE /{repo}/-/package/{package}/dist-tags/{tag}`
- **Revision-aware metadata update or unpublish**: `PUT /{repo}/{package}/-rev/{revision}`
- **Revision-aware package deletion**: `DELETE /{repo}/{package}/-rev/{revision}`

Versions are immutable. Unpublish and deletion create tombstones, so a published semantic version cannot be reused.
Revision conflicts return `409 Conflict` and require the client to fetch the current packument.

## Browser management API

The same-origin management endpoints use JSON and return a stable `X-Renop-Error-Code` header on failure.

- `GET /api/npm/repositories/{repo}/packages`
- `POST /api/npm/repositories/{repo}/packages`
- `PUT /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/versions?package={package}&version={version}`
- `GET /api/npm/repositories/{repo}/owners?package={package}`
- `POST /api/npm/repositories/{repo}/owners?package={package}`
- `PUT /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `DELETE /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `GET /api/npm/repositories/{repo}/users/search?package={package}&q={query}`
- `POST /api/npm/repositories/{repo}/invitations/{id}/{accept|reject}`

Catalog responses are paginated with `limit` from 1 to 100 and a bounded `offset`. Private packages are omitted unless
the caller has package membership or administrator access. Team details are returned only to L3/L4 members and
administrators.

The package-detail response includes bounded README, author, contributor, maintainer, license, runtime, keyword, and
project-link metadata from the selected published version. The browser renders README Markdown through an element and
URL allowlist; package-controlled HTML and unsafe links are never activated.

## Authentication and authorization

npm clients may use Basic authentication with an account password or API Token, or an API Token as `_authToken`.
Bearer API Token scopes are intersected with the account's current permissions and optional exact repository, package,
or team targets. Publication still requires an existing package and L1 or higher; package metadata and unpublish need
L2, team changes need L3, and ownership or package deletion needs L4.
