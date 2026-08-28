---
title: API Index
order: 1
category: API Reference
description: RenoP HTTP RESTful and RPC API overview and endpoints
---

# RenoP HTTP API

RenoP provides a complete HTTP API for administrative automation, client integrations, and health monitoring. The server
listens on `http://localhost:3000` by default.

## API Route Structure

| Route Prefix                    | Purpose                                                              |
|:--------------------------------|:---------------------------------------------------------------------|
| `/api/*`                        | Management APIs (authentication, tokens, settings, status, messages) |
| `/{repo}/*`                     | Maven/files storage or format-specific package protocol              |
| `/{npm-repo}/*`                 | npm packuments, tarballs, publication, dist-tags, and search         |
| `/index/*` or `/{repo}/index/*` | Cargo Sparse Index endpoints                                         |
| `/v2/*`                         | Docker & OCI Distribution Spec v2 endpoints                          |
| `/javadoc/*`                    | Javadoc online HTML viewer                                           |
| `/cargodoc/*`                   | Cargodoc online HTML viewer                                          |

## Wire Formats & Protobuf

Most endpoints consume and produce JSON. High-throughput endpoints additionally support Google Protocol Buffers
(`application/x-protobuf`).

Pass `Accept: application/x-protobuf` or `Content-Type: application/x-protobuf` in request headers to utilize binary
serialization. Proto definitions are located in `proto/api/v1/api.proto`.

## Authentication Transports

- **Browser cookie**: `renop_session=<session_id>`; the HttpOnly session secret is not accepted in headers or URLs.
- **Bearer API token**: `Authorization: Bearer <token>`; endpoint scopes are intersected with account permissions.
- **Basic Auth for package protocols**: `Authorization: Basic <base64(user:password_or_token)>`.

Basic credentials cannot call management APIs. Query-string credentials and `Authorization: Session` are rejected.

## HTTP Status Codes

| Code                      | Meaning      | Description                                       |
|:--------------------------|:-------------|:--------------------------------------------------|
| `200 OK`                  | Success      | Request succeeded with response body              |
| `201 Created`             | Created      | Resource or upload task successfully initialized  |
| `204 No Content`          | Success      | Request succeeded with no response body           |
| `400 Bad Request`         | Bad Request  | Invalid parameters or body format                 |
| `401 Unauthorized`        | Unauthorized | Missing or invalid authentication credentials     |
| `403 Forbidden`           | Forbidden    | Insufficient permissions or IP temporarily banned |
| `404 Not Found`           | Not Found    | Resource not found                                |
| `409 Conflict`            | Conflict     | Artifact already exists and cannot be overwritten |
| `429 Too Many Requests`   | Rate Limited | Request rate exceeds configured limits            |
| `503 Service Unavailable` | Overloaded   | Maximum active concurrent requests reached        |

## API Reference Index

- [Authentication API](./authentication.md)
- [Tokens & Users API](./tokens.md)
- [Maven Metadata API](./maven.md)
- [Cargo Registry API](./cargo.md)
- [Docker / OCI Registry API](./docker.md)
- [npm Registry API](./npm.md)
- [Message Center API](./messages.md)
- [Storage & Upload API](./storage.md)
- [Settings API](./settings.md)
- [Status & Telemetry API](./status.md)
- [GPG Cryptography API](./gpg.md)
- [Rate Limiting Reference](./rate-limit.md)
- [Updater API](./updater.md)
