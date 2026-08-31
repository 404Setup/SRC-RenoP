---
title: Troubleshooting
order: 5
category: Guides
description: A status-first workflow for startup, authentication, proxy, protocol, mirror, and storage failures
---

# Troubleshooting

Start with the HTTP status, the exact URL, the repository format and visibility, and the credential type. Avoid changing
multiple settings at once: a package client can hide the original server response behind its own generic message.

## Collect the minimum evidence

Record the following before restarting or deleting state:

- RenoP version and startup time;
- request timestamp, method, sanitized URL, and response status;
- repository name, format, visibility, and whether a mirror or publication review applies;
- client name and version, command, and a verbose log with secrets removed;
- relevant server log lines and, when present, `X-Renop-Error-Code`;
- database and storage reachability, free space, and recent configuration changes.

Never paste session cookies, API-token secrets, passwords, S3 keys, OAuth secrets, or complete authorization headers
into
an issue or chat transcript.

## The process does not start

Check the configured paths and working directory first. Relative paths for `config.yaml`, `repositories.yaml`, the
SQLite database, `index.json`, and local storage are resolved from the service's working environment, which can differ
from an interactive shell.

Common causes are an occupied listener port, malformed YAML, an unreachable database DSN, missing write permission,
invalid TLS files, or a service account that cannot read secrets. The initial admin password affects only account
bootstrap; changing `RENOP_DEFAULT_ADMIN_PASSWORD` does not reset an existing administrator.

## Health is up but the application fails

```bash
curl -i https://packages.example.com/api/status/health
```

`"UP"` confirms that the process is serving the health route. It does not validate login, database writes, local or S3
storage, mirror access, or publication policy. Continue with an authenticated browser request and a disposable package
operation against the affected backend.

If the web UI reports that a newer interface is available, reload it before debugging protobuf decode or missing-route
errors. A reverse proxy or browser cache may otherwise keep JavaScript from a different server version.

## Interpret the status before the message

| Status | First checks                                                                                                 |
|:-------|:-------------------------------------------------------------------------------------------------------------|
| `400`  | Malformed protobuf/JSON, invalid path or name, missing required field, unsupported operation                 |
| `401`  | Missing, expired, malformed, or disallowed credential type; cookie not returned through HTTPS/proxy          |
| `403`  | Account permission, token scope/target, team level, repository visibility, debug mode, or review role        |
| `404`  | Wrong repository/path, hidden resource, absent version, mirror miss, or intentionally concealed private data |
| `409`  | Immutable version/tag conflict, existing reservation, state transition conflict, or concurrent decision      |
| `413`  | Reverse-proxy or server upload limit; verify the layer or artifact size and buffering settings               |
| `429`  | Rate or concurrency control; honor retry guidance and reduce parallel work                                   |
| `5xx`  | Database, storage, upstream, signing, extraction, or internal failure; inspect the server log                |

Plain-text error sentences are for humans and may change. Use the status, protocol-native structured body, and stable
error header when one is provided.

## Authentication and browser sessions

The management UI uses the HttpOnly `renop_session` cookie. Private account-security endpoints do not accept a password,
Bearer token, `Authorization: Session`, or a session value in the URL. Confirm that the public origin is HTTPS, the
proxy forwards the original scheme and host, and the browser is allowed to return the cookie to the same origin.

For automation, use a scoped Bearer API token. Its effective access is the intersection of token scopes, target
restrictions, the account's current permission, repository policy, and package-team membership. Reissuing a broader
token does not repair a missing account or team permission.

## Maven and Gradle

- Confirm that the repository URL ends at the RenoP repository name, not at `/api`.
- Match the Maven `<server><id>` to the repository ID used by `distributionManagement` or the dependency repository.
- Use the account name as the Basic username and a scoped API token as the password.
- Verify that the `groupId` is under a publishing domain controlled by the account and that the required team level is
  present.
- For signed repositories, upload the required detached signature and check the backend signing record rather than
  relying on a filename alone.
- A release redeployment or other immutable publication should fail; do not work around it by deleting server files.

## Cargo

- Use a sparse URL with the repository path and trailing slash, for example
  `sparse+https://packages.example.com/crates/`.
- Run `cargo login --registry <name>` and store the complete RenoP token value expected by Cargo.
- Distinguish `repository:publish`, `package:create`, lifecycle, and team-management scopes.
- A first publication can fail safely when the upstream name check is unavailable; retry after upstream connectivity is
  restored rather than assuming the name was reserved.
- While publication review is pending, the accepted archive is not yet visible in the sparse index or public catalog.

## npm

- Set the registry to the repository path, not only the host. Configure a separate scoped registry when required.
- Check the exact token entry written to the user or CI `.npmrc`, and do not commit it with the project.
- Reserve a package before the first publication when repository policy requires it.
- Version publication is immutable. A conflicting version is not repaired by retrying with higher concurrency.
- For a mirrored package, determine whether the requested version is upstream content or a locally owned package before
  changing team or dist-tag settings.

## Docker and OCI

- Log in to the registry host; image names and repository paths are supplied to `pull`, `push`, or Podman separately.
- Use a certificate trusted by the client. Configure an insecure registry only for an isolated test environment.
- Create or reserve the image/namespace required by RenoP policy before the first push.
- Preserve the `/v2/` challenge and `/v2/token` exchange through the reverse proxy. Stripping `Authorization` or
  rewriting paths breaks the Bearer flow.
- When a push fails, identify whether the rejected object is a blob, manifest, or tag and compare its digest and media
  type with the server response.

## Mirrors, storage, and reverse proxies

A mirror miss can be an upstream `404`, a negative-cache hit, an allowlist denial, an expired credential, a proxy
failure, or a local commit failure. Compare a direct upstream request from the RenoP host with the same request through
RenoP, without bypassing authorization in production.

For S3-compatible storage, verify endpoint, region, path style, bucket, prefix, clock synchronization, TLS trust, and
read/write/list/delete permissions. For presigned redirects, test the returned URL from the client network. For local
storage, check ownership, free space, temporary-file capacity, and filesystem atomic-rename behavior.

For large uploads, disable request buffering, remove body-size limits, and extend read/write timeouts. Trust forwarding
headers only from configured proxies; otherwise rate limits and audit IPs can be forged.

## Escalate with a reproducible case

Reduce the failure to one repository, one disposable package, and one command. Include redacted configuration sections,
expected and actual status, and whether the request works directly against RenoP without the reverse proxy. State which
recovery actions were already attempted. Do not delete the database, storage prefix, or package ownership to make the
symptom disappear before evidence has been captured.

For API-specific client behavior, see [HTTP API Integration](../api/client-integration.md). For deployment validation,
use the [Production Deployment Checklist](../deployment/production-checklist.md).
