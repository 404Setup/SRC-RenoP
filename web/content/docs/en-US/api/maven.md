---
title: Maven
order: 4
category: API
---

# Maven browse and helpers

Prefix: `/api/maven` (badge under `/api/badge`)

These endpoints read index and metadata. Actual artifact bytes live at `/{repo}/group/artifact/…` —
see [storage.md](./storage.md).

Path parameters use Maven layout, e.g.:

```text
com/example/demo
com/example/demo/1.0.0
```

Insufficient read permission usually yields `404 Not found`.

## Directory and file details (Protobuf)

### `GET /api/maven/details`

Repositories visible to the current user, wrapped as a virtual root.

Response: `FileDetails` (`application/x-protobuf`)

```text
type = DIRECTORY
name = "repositories"
files[] = { type: DIRECTORY, name: "<repo>" }
```

### `GET /api/maven/details/:repo_name`

Repository root (with children).

### `GET /api/maven/details/:repo_name/*`

Path details. Directories include `files`; files include `content_length` and `last_modified_time` (RFC3339Nano).

`type` is `FILE` or `DIRECTORY`.

### `GET /api/maven/repo-details/:repo_name`

Stats and mirror summary. Response: `RepoDetailsResponse`.

| Field                                               | Meaning                                                 |
|-----------------------------------------------------|---------------------------------------------------------|
| `name` / `visibility`                               | Name, visibility                                        |
| `total_size` / `artifact_size` / `metadata_size`    | Bytes                                                   |
| `total_files` / `artifact_count` / `metadata_count` | Counts (checksums and maven-metadata count as metadata) |
| `mirrors[]`                                         | name, url, persist, cache_ttl, negative_cache, …        |

No read access → **403** (unlike details, which often use 404).

## Version queries (JSON)

Path should point at a coordinate directory that has `maven-metadata.xml` (groupId/artifactId).

### `GET /api/maven/versions/:repo_name/*`

| Query    | Default | Meaning                  |
|----------|---------|--------------------------|
| `filter` | —       | Version substring filter |
| `sorted` | `true`  | Sort results             |

```json
{
  "is_snapshot": false,
  "versions": ["1.0.0", "1.1.0"]
}
```

### `GET /api/maven/latest/version/:repo_name/*`

Same query params; add `type=raw` for a bare version string.

Otherwise:

```json
{
  "is_snapshot": false,
  "version": "1.1.0"
}
```

### `GET /api/maven/latest/details/:repo_name/*`

`FileDetails` for the latest matching artifact (**JSON**, not protobuf).

| Query        | Default | Meaning        |
|--------------|---------|----------------|
| `extension`  | `jar`   | Extension      |
| `classifier` | —       | Classifier     |
| `filter`     | —       | Version filter |

### `GET /api/maven/latest/file/:repo_name/*`

Resolve latest version, then fetch via the storage layer (redirect or body — similar to a direct artifact URL).

## Badge

### `GET /api/badge/latest/:repo_name/*`

SVG badge with the latest version. `Content-Type: image/svg+xml`.

| Query    | Meaning                               |
|----------|---------------------------------------|
| `name`   | Left label (default: repository name) |
| `color`  | Right color (alphanumeric or `#hex`)  |
| `prefix` | Version prefix text                   |
| `filter` | Version filter                        |

```markdown
![latest](https://your-host/api/badge/latest/releases/com/example/demo)
```

## Generate POM

### `POST /api/maven/generate/pom/:repo_name/*`

Requires write access to the repository.

```json
{
  "group_id": "com.example",
  "artifact_id": "demo",
  "version": "1.0.0"
}
```

Path may already end in `.pom`, or be a coordinate directory (then filename is `artifact_id-version.pom`).

Insufficient disk → 507. On success the POM is written and the index updated.

## Privacy policy

### `GET|HEAD /api/privacy-policy`

If `privacy-policy.txt` exists in the working directory, return it as `text/plain`; otherwise 404. Unrelated to Maven;
mounted on the same API group.
