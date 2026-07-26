---
title: Storage
order: 8
category: API
---

# Repository storage paths

Artifacts are not under `/api`. Layout:

```text
/{repo_name}/{maven-path}
```

Default repositories:

```text
/releases/...
/snapshots/...
/private/...
```

Repository names must not collide with static routes: `api`, `js`, `css`, `svg`, `assets`, `javadocs`, etc.

## Methods

| Method     | Permission | Behavior                                                            |
|------------|------------|---------------------------------------------------------------------|
| GET        | read       | Download; browser HTML requests may fall back to the management SPA |
| HEAD       | read       | Headers only                                                        |
| PUT / POST | write      | Upload / overwrite                                                  |
| DELETE     | write      | Delete; success 204                                                 |

Body limit is about 2 GiB (`BodyLimit`); uploads stream.

### Upload

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Typical success: `201 Created`. If redeploy is disabled and the file exists, the server rejects overwrite (treat any
non-2xx as failure).

Optional header: `X-Generate-Checksums: true` writes `.md5` / `.sha1` / `.sha256` / `.sha512` sidecars.

The server maintains index, optional checksums, and S3 sync. Maven clients see a normal repository layout.

### Multi-part (chunked) upload — optional

The original single-request `PUT` above is unchanged. For large files the web UI may use concurrent chunked upload
instead (with per-part retries). Machine clients can use the same API.

**When to use multi-part:** the browser UI skips chunking for files under **8 MiB** (single `PUT` is faster). Machine
clients may still open a chunked session for any size; the server will collapse very small payloads into one part.

Prefix: `/api/upload/chunked` (session cookie / Basic / Bearer; needs write permission on the target repo).

Init and complete use **`application/x-protobuf`** (`ChunkedUploadInitRequest` /
`ChunkedUploadInitResponse` / `ChunkedUploadCompleteResponse` in `proto/api/v1/api.proto`). Part bodies are raw binary.

1. **`POST /api/upload/chunked/`** — start a session (`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`)

Logical fields (snake_case):

| Field                | Meaning                                           |
|----------------------|---------------------------------------------------|
| `purpose`            | `storage` (default)                               |
| `path`               | Destination `repo/…/file`                         |
| `filename`           | Optional display name                             |
| `size`               | Total bytes                                       |
| `generate_checksums` | Write checksum sidecars                           |
| `chunk_size`         | Preferred part size (optional; server normalizes) |

Response fields: `upload_id`, `chunk_size`, `chunk_count`, `purpose`. Always use the returned
`chunk_size` / `chunk_count` for subsequent `PUT`s.

**Part size rules** (server, `upload.NormalizeChunkSize`):

| Total size | Typical part size       |
|------------|-------------------------|
| ≤ 256 KiB  | Single part = file size |
| ≤ 8 MiB    | Single part = file size |
| ≤ 32 MiB   | 4 MiB                   |
| ≤ 128 MiB  | 8 MiB                   |
| ≤ 512 MiB  | 16 MiB                  |
| ≤ 2 GiB    | 24 MiB                  |
| larger     | 32 MiB (max)            |

Client `chunk_size` is clamped to **256 KiB … 32 MiB**. If it would create more than ~2048 parts, the server raises the
part size. Omit `chunk_size` (or send `0`) to accept the table above.

2. **`PUT /api/upload/chunked/:upload_id/:index`** — raw part body (0-based), parallel OK  
   Success: `204`. Re-PUT of an already accepted index is idempotent (retry-safe).

3. **`POST /api/upload/chunked/:upload_id/complete`** — assemble, index, optional checksums  
   Success: `201` + `ChunkedUploadCompleteResponse` (`status=created`, `path=…`).

4. **`DELETE /api/upload/chunked/:upload_id`** — abort and discard temp data (`204`).

Sessions expire after about **15 minutes** if not completed (temps are removed). Clients should retry failed parts with
backoff.

### Download

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC needs no auth. PRIVATE uses Basic / Bearer.

With mirrors configured, missing local files may be fetched from upstream (cache / negative-cache per repository
config).

### Delete

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## Browser access

With `Accept: text/html`, missing repositories or some directories fall through to the management SPA so
`http://host/releases/...` can open the UI. Machine clients should use `Accept: */*` or omit Accept to avoid HTML.

## Javadoc preview

When enabled:

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

Needs matching read permission. `raw` serves files inside the jar. Size limited by `max_javadoc_size_mb`.

## Maven configuration example

```xml

<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>

<distributionManagement>
<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>
<snapshotRepository>
    <id>renop</id>
    <url>http://localhost:3000/snapshots</url>
</snapshotRepository>
</distributionManagement>
```

In `~/.m2/settings.xml`, set username + password (or upload token) for the server id.
