---
title: API index
order: 1
category: API
---

# RenoP HTTP API

Default listen address: `0.0.0.0:3000`.

| Path        | Purpose                                              |
|-------------|------------------------------------------------------|
| `/api/*`    | Management APIs (login, settings, status, …)         |
| `/{repo}/…` | Maven repository layout (download / upload / delete) |

Error bodies are often plain text (`Unauthorized`, `Forbidden`, `Not found`). Trust the status code first.

## Index

| File                                     | Contents                                                    |
|------------------------------------------|-------------------------------------------------------------|
| [authentication.md](./authentication.md) | Login, sessions, permissions                                |
| [tokens.md](./tokens.md)                 | Account management (manager)                                |
| [maven.md](./maven.md)                   | Browse, versions, badge, generate POM                       |
| [status.md](./status.md)                 | Health and runtime status                                   |
| [settings.md](./settings.md)             | Config domains, repositories, index rebuild                 |
| [updater.md](./updater.md)               | Online / offline updates                                    |
| [storage.md](./storage.md)               | GET/PUT/DELETE on repository paths; optional chunked upload |
| [rate-limit.md](./rate-limit.md)         | IP rate limits, auth-failure ban, concurrent request cap    |

Machine-readable schema: [openapi.yaml](/assets/openapi.yaml).  
Proto definitions: `proto/api/v1/api.proto` (generated Go code under `pb/`).

## JSON and Protobuf

Most endpoints still use JSON. These use `application/x-protobuf`:

| Endpoint                                     | Direction          |
|----------------------------------------------|--------------------|
| `POST /api/auth/login`                       | request + response |
| `GET /api/auth/me`                           | response           |
| `GET /api/tokens`                            | response           |
| `GET /api/status/instance`                   | response           |
| `GET /api/status/snapshots`                  | response           |
| `GET /api/updater/status`                    | response           |
| `POST /api/settings/index/rebuild`           | request            |
| `GET /api/settings/domains`                  | response           |
| `GET /api/settings/domain/:name`             | response           |
| `PUT /api/settings/domain/:name`             | request            |
| `GET /api/settings/maven/repositories`       | response           |
| `PUT /api/settings/maven/repositories/:name` | request            |
| `GET /api/maven/details…`                    | response           |
| `GET /api/maven/repo-details/:repo`          | response           |
| `POST /api/upload/chunked/`                  | request + response |
| `POST /api/upload/chunked/:id/complete`      | response           |

Field names match the proto (snake_case). Generate clients with `protoc`, or follow the frontend `protobufjs` codecs.

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

```bash
# After login, cookie name is renop_session
curl -s -b 'renop_session=<session-id>' \
  -H 'Accept: application/x-protobuf' \
  http://localhost:3000/api/auth/me \
  -o me.bin
```

## Authentication

Supported carriers:

1. Cookie: `renop_session=<id>`
2. `Authorization: Session <id>`
3. `Authorization: Basic base64(user:password_or_upload_token)`
4. `Authorization: Bearer <user>:<secret>` or `Bearer <upload-token>`
5. GET/HEAD only: `?token=<session-or-bearer>`

Sessions expire after about **7 days** of idle time and renew on activity.

| Role            | Capabilities                                             |
|-----------------|----------------------------------------------------------|
| Anonymous       | Read PUBLIC repositories; management APIs mostly 401/403 |
| Regular user    | Access repositories via `canview:` / `canupdate:`        |
| manager / admin | User, settings, updater, and other management APIs       |

Details: [authentication.md](./authentication.md).

## Status codes

| Code | Meaning                                                   |
|------|-----------------------------------------------------------|
| 200  | OK (body may be empty or plain text)                      |
| 201  | Upload created                                            |
| 204  | Success, no body                                          |
| 400  | Bad parameters / body                                     |
| 401  | Not authenticated or invalid credentials                  |
| 403  | Not allowed, expired, or IP banned after repeated 401/403 |
| 404  | Missing; private reads may also return 404 instead of 403 |
| 409  | Conflict (name taken, update already running)             |
| 429  | Anonymous IP exceeded request rate limit                  |
| 503  | Overloaded (e.g. concurrent request cap)                  |
| 507  | Insufficient disk space                                   |

Rate limits and anomaly rules: [rate-limit.md](./rate-limit.md).

Instance version: `version` on `GET /api/status/instance`. There is no separate API version field.
