---
title: HTTP API Integration
order: 19
category: API Reference
description: API-family selection, protobuf media types, credentials, errors, retries, and client compatibility
---

# HTTP API Integration

RenoP serves management endpoints and several package-client protocols from the same origin. Select the API family
before choosing a media type or credential; treating every route as a JSON REST endpoint is incorrect.

## Choose the correct API surface

| Surface                    | Typical paths                               | Intended client                             |
|:---------------------------|:--------------------------------------------|:--------------------------------------------|
| Management and browser API | `/api/...`                                  | RenoP UI, administrative tools, automation  |
| Maven and generic files    | `/{repo}/{path}`                            | Maven, Gradle, HTTP artifact clients        |
| Cargo sparse registry      | `/{repo}/config.json`, `/{repo}/api/v1/...` | Cargo and compatible tooling                |
| npm registry               | `/{repo}/{package}`, `/{repo}/-/...`        | npm-compatible clients                      |
| Docker/OCI Distribution    | `/v2/...`, `/v2/token`                      | Docker, Podman, OCI clients                 |
| Documentation previews     | `/javadoc/...`, `/cargodoc/...`             | Web browsers after repository authorization |

Do not prepend `/api` to a native package-client URL. Conversely, do not infer management semantics from a package
protocol's method or error shape.

## Use the declared representation

Most management requests and responses use `application/x-protobuf`. The OpenAPI document describes logical fields for
human reference and tooling, but those schema examples do not make the endpoint JSON-compatible. Use the protobuf
messages from `proto/api/v1/api.proto` that belong to the same RenoP release.

Set both headers explicitly when an endpoint has a protobuf body:

```http
Content-Type: application/x-protobuf
Accept: application/x-protobuf
```

Health checks and some errors are plain text. Cargo, npm, and Docker/OCI routes use the structured JSON or binary
formats
required by those protocols. Always follow the endpoint documentation instead of guessing from a path suffix.

## Select the credential by caller

| Credential             | Use                                                            | Important restriction                                                     |
|:-----------------------|:---------------------------------------------------------------|:--------------------------------------------------------------------------|
| `renop_session` cookie | Interactive browser UI and private account-security operations | HttpOnly; do not extract or replay it in scripts                          |
| Bearer API token       | Management automation and supported API-token routes           | Effective access is intersected with current account and team permissions |
| HTTP Basic             | Package clients and designated upload flows                    | Not a general replacement for the browser session or Bearer token         |
| Docker Bearer token    | Docker/OCI Distribution operations                             | Obtained through the registry challenge and token exchange                |

A token secret should be shown only at creation. Store it in a secret manager, set an expiration, restrict targets and
scopes, and revoke it when the job or device is retired. Query-string credentials and `Authorization: Session` are
rejected.

## Build the base URL correctly

Use one canonical HTTPS origin in production. Preserve the original `Host` and scheme through the reverse proxy so that
cookies, redirects, Docker challenges, and generated repository URLs refer to the public service.

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

A successful response body is `"UP"`.

The health endpoint is suitable for reachability checks, not for proving that the database or storage can commit data.
Use a separate authenticated readiness check in deployment automation when those dependencies must be validated.

## Handle responses in a stable order

1. Read the HTTP status.
2. Inspect the response `Content-Type`.
3. Read `X-Renop-Error-Code` when present.
4. Decode the protocol-native body only with the matching decoder.
5. Log a correlation timestamp and sanitized request context, never the credential.

Management failures may contain short plain-text explanations. Docker Distribution, Cargo, and npm retain their native
structured error forms. Do not branch on a complete English sentence.

## Map status to client behavior

| Status      | Client action                                                                                             |
|:------------|:----------------------------------------------------------------------------------------------------------|
| `200`–`204` | Decode according to the documented response type; a successful empty body is valid where specified        |
| `202`       | Treat the operation as accepted but not necessarily visible; publication review may still be pending      |
| `302`       | Follow only for documented downloads, such as an authorized S3 presigned redirect                         |
| `400`       | Correct the request; automatic retry normally repeats the same failure                                    |
| `401`       | Refresh or replace the credential only after checking that the credential type is allowed                 |
| `403`       | Do not retry blindly; scopes, targets, account permissions, team level, policy, or debug mode must change |
| `404`       | Verify path and visibility; private or hidden data may intentionally be concealed                         |
| `409`       | Re-read state before deciding whether the immutable or concurrent operation can be changed                |
| `413`       | Reduce the payload only when valid, otherwise fix proxy/server limits                                     |
| `429`       | Respect retry guidance, add jitter, and lower concurrency                                                 |
| `5xx`       | Retry only bounded, safe operations; preserve the original error and inspect service dependencies         |

## Retry only when semantics allow it

GET and HEAD are generally safe to retry after transport failure. For writes, determine whether the endpoint or package
protocol is idempotent and whether the server may have committed the request before the connection was lost. Use
bounded exponential backoff with jitter and a total deadline.

Never retry an immutable publication by silently changing a version, deleting data, or broadening credentials. For
chunked or registry uploads, continue through the protocol's own upload state rather than replaying unrelated steps.

## Respect endpoint-specific pagination and filters

List endpoints do not share one universal cursor or page model. Use the parameters documented for that endpoint, retain
stable identifiers returned by the server, and stop when the returned page indicates completion. Apply filters on the
server where supported, but do not assume that a UI filter changes authorization or repository visibility.

## Keep contracts from one release

Generate clients from `web/assets/openapi.yaml` and `proto/api/v1/api.proto` from the same commit or release as the
server. An OpenAPI field name describes the logical protobuf field and may not match a JSON wire representation. Native
Maven, Cargo, npm, and Docker clients should continue to use their protocol configuration rather than generated
management stubs.

Before upgrading production, run contract tests against a non-production instance for login, token authorization,
repository listing, one representative read and write per enabled format, pagination, error decoding, and reverse-proxy
behavior.

## Integration checklist

- [ ] Correct API family and repository base path selected.
- [ ] HTTPS origin, proxy host, and scheme are canonical.
- [ ] Request and response media types are explicit.
- [ ] Credential type is allowed for the target route.
- [ ] Token scope, target, expiration, and owner permission are minimal and current.
- [ ] Status is handled before body text.
- [ ] Retries are bounded, jittered, and safe for the operation.
- [ ] Logs redact cookies, passwords, tokens, and signed URLs.
- [ ] OpenAPI and protobuf files match the deployed release.
- [ ] A protocol-native end-to-end test runs before deployment.

See [Authentication API](./authentication.md), [API Tokens & Users](./tokens.md), and
[Troubleshooting](../guides/troubleshooting.md) for route-level details and failure diagnosis.
