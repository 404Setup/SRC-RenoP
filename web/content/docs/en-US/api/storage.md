---
title: Storage
order: 8
category: API
---

# Repository storage paths

Artifact paths are not under `/api`. Layout:

```text
/{repo_name}/{maven-path}
```

Default repositories:

```text
/releases/...
/snapshots/...
/private/...
```

Repository names must not collide with static routes such as `api`, `js`, `css`, `svg`, `assets`, or `javadocs`.

## Methods

| Method     | Permission | Behavior                                                              |
|------------|------------|-----------------------------------------------------------------------|
| GET        | read       | Download; browser requests with HTML Accept may fall back to the SPA  |
| HEAD       | read       | Response headers only                                                 |
| PUT / POST | write      | Upload or overwrite                                                   |
| DELETE     | write      | Delete; success status `204`                                          |

Maximum body size is approximately 2 GiB (`BodyLimit`). Uploads are streamed.

### Upload

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Successful upload returns `201 Created`. If redeployment is disabled and the object already exists, the request fails with a non-2xx status.

Optional request header `X-Generate-Checksums: true` writes `.md5`, `.sha1`, `.sha256`, and `.sha512` sidecar files.

The server updates the artifact index, optional checksums, and S3 synchronization as configured. Clients observe a standard Maven repository layout.

### Chunked upload (optional)

Authentication matches storage write: session cookie, Basic, or Bearer, with write permission on the target repository.

Prefix: `/api/upload/chunked`

The browser UI uses chunked upload for files of **8 MiB** or larger; smaller files use a single `PUT`. Non-browser clients may open a chunked session for any size. The server may combine very small payloads into a single part.

Init and complete use **`application/x-protobuf`** (`ChunkedUploadInitRequest`, `ChunkedUploadInitResponse`, and `ChunkedUploadCompleteResponse` in `proto/api/v1/api.proto`). Part bodies are raw binary.

1. **`POST /api/upload/chunked/`** — create a session (`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`)

| Field                | Description                                      |
|----------------------|--------------------------------------------------|
| `purpose`            | `storage` (default)                              |
| `path`               | Destination path `repo/…/file`                   |
| `filename`           | Optional display name                            |
| `size`               | Total size in bytes                              |
| `generate_checksums` | Whether to write checksum sidecars               |
| `chunk_size`         | Preferred part size (optional; server normalizes)|

Response fields: `upload_id`, `chunk_size`, `chunk_count`, `purpose`. Subsequent part uploads must use the returned `chunk_size` and `chunk_count`.

**Part size rules** (server, `upload.NormalizeChunkSize`):

| Total size | Part size                        |
|------------|----------------------------------|
| ≤ 256 KiB  | Single part equal to file size   |
| ≤ 8 MiB    | Single part equal to file size   |
| ≤ 32 MiB   | 4 MiB                            |
| ≤ 128 MiB  | 8 MiB                            |
| ≤ 512 MiB  | 16 MiB                           |
| ≤ 2 GiB    | 24 MiB                           |
| larger     | 32 MiB (maximum)                 |

Client-supplied `chunk_size` is clamped to **256 KiB … 32 MiB**. If the resulting part count would exceed approximately 2048, the server increases the part size. Omit `chunk_size` or send `0` to use the table above.

2. **`PUT /api/upload/chunked/:upload_id/:index`** — raw part body (0-based index); parallel uploads allowed  
   Success: `204`. Re-uploading an already accepted index is idempotent.

3. **`POST /api/upload/chunked/:upload_id/complete`** — assemble, update index, optional checksums  
   Success: `201` with `ChunkedUploadCompleteResponse` (`status=created`, `path=…`).

4. **`DELETE /api/upload/chunked/:upload_id`** — abort the session and discard temporary data (`204`).

Incomplete sessions expire after approximately **15 minutes**; temporary data is removed.

### Download

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC repositories require no authentication. PRIVATE repositories require Basic or Bearer credentials.

When mirrors are configured, missing local objects may be fetched from upstream according to per-repository cache and negative-cache settings.

### Delete

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## Browser access

With `Accept: text/html`, missing repositories or some directories fall through to the management SPA so paths such as `http://host/releases/...` can open the UI. Machine clients should send `Accept: */*` or omit `Accept` to avoid HTML responses.

## Javadoc preview

When enabled:

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

Requires matching read permission. The `raw` form serves files from inside the jar. Size is limited by `max_javadoc_size_mb`.

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

In `~/.m2/settings.xml`, set username and password (or upload token) for the matching server `id`.
