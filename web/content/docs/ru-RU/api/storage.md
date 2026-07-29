---
title: Storage
order: 8
category: API
---

# Пути хранилища репозиториев

Пути артефактов не находятся под `/api`. Layout:

```text
/{repo_name}/{maven-path}
```

Репозитории по умолчанию:

```text
/releases/...
/snapshots/...
/private/...
```

Имена репозиториев не должны пересекаться с префиксами статических маршрутов, такими как `api`, `js`, `css`, `svg`,
`assets` или `javadoc`.

## Methods

| Method     | Permission | Behavior                                                              |
|------------|------------|-----------------------------------------------------------------------|
| GET        | read       | Download; browser-запросы с HTML Accept могут упасть в management SPA |
| HEAD       | read       | Только response headers                                               |
| PUT / POST | write      | Upload или overwrite                                                  |
| DELETE     | write      | Delete; success status `204`                                          |

Максимальный размер body примерно 2 GiB (`BodyLimit`). Upload выполняется потоком.

### Upload

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Успешный upload возвращает `201 Created`. Если redeploy отключён и объект уже существует, ответ — `409 Conflict`.

Опциональный request header `X-Generate-Checksums: true` записывает sidecar-файлы `.md5`, `.sha1`, `.sha256` и
`.sha512`.

Сервер обновляет artifact index, optional checksums и S3 sync согласно конфигурации. Клиенты видят стандартный Maven
repository layout.

### Chunked upload (optional)

Аутентификация совпадает с storage write: session cookie, Basic или Bearer, с write permission на target repository.

Prefix: `/api/upload/chunked`

Browser UI использует chunked upload для файлов **8 MiB** и больше; меньшие файлы идут одним `PUT`. Non-browser clients
могут открыть chunked session любого размера. Сервер может объединить очень малые payload в одну part.

Init и complete используют **`application/x-protobuf`** (`ChunkedUploadInitRequest`, `ChunkedUploadInitResponse` и
`ChunkedUploadCompleteResponse` в `proto/api/v1/api.proto`). Part bodies — raw binary.

1. **`POST /api/upload/chunked/`** — создать session (`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`)

| Field                | Description                                       |
|----------------------|---------------------------------------------------|
| `purpose`            | `storage` (default)                               |
| `path`               | Destination path `repo/…/file`                    |
| `filename`           | Optional display name                             |
| `size`               | Total size in bytes                               |
| `generate_checksums` | Whether to write checksum sidecars                |
| `chunk_size`         | Preferred part size (optional; server normalizes) |

Response fields: `upload_id`, `chunk_size`, `chunk_count`, `purpose`. Последующие part uploads должны использовать
returned `chunk_size` и `chunk_count`.

**Правила размера part** (server, `upload.NormalizeChunkSize`):

| Total size | Part size                      |
|------------|--------------------------------|
| ≤ 256 KiB  | Single part equal to file size |
| ≤ 8 MiB    | Single part equal to file size |
| ≤ 32 MiB   | 4 MiB                          |
| ≤ 128 MiB  | 8 MiB                          |
| ≤ 512 MiB  | 16 MiB                         |
| ≤ 2 GiB    | 24 MiB                         |
| larger     | 32 MiB (maximum)               |

Client-supplied `chunk_size` ограничен **256 KiB … 32 MiB**. Если число parts превысило бы примерно 2048, сервер
увеличивает part size. Опустите `chunk_size` или отправьте `0`, чтобы использовать таблицу выше.

2. **`PUT /api/upload/chunked/:upload_id/:index`** — raw part body (0-based index); parallel uploads разрешены  
   Success: `204`. Повторная загрузка уже accepted index идемпотентна.

3. **`POST /api/upload/chunked/:upload_id/complete`** — assemble, update index, optional checksums  
   Success: `201` с `ChunkedUploadCompleteResponse` (`status=created`, `path=…`).

4. **`DELETE /api/upload/chunked/:upload_id`** — abort session и discard temporary data (`204`).

Incomplete sessions истекают примерно через **15 minutes**; temporary data удаляется.

### Download

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC repositories не требуют authentication. PRIVATE требуют Basic или Bearer.

При настроенных mirrors отсутствующие локально objects могут быть получены с upstream согласно per-repository cache и
negative-cache.

### Delete

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## Browser access

С `Accept: text/html` отсутствующие repositories или некоторые directories fall through в management SPA, поэтому пути
вида `http://host/releases/...` могут открыть UI. Machine clients должны отправлять `Accept: */*` или опускать `Accept`,
чтобы не получать HTML.

## Javadoc preview

Когда включено:

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

Требуется matching read permission. Форма `raw` отдаёт files внутри jar. Size ограничен `max_javadoc_size_mb`.

## Пример конфигурации Maven

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

В `~/.m2/settings.xml` задайте username и password (или upload token) для matching server `id`.
