---
title: Хранилище
order: 8
category: API
---

# Пути хранилища репозиториев

Артефакты не под `/api`. Раскладка:

```text
/{repo_name}/{maven-path}
```

Репозитории по умолчанию:

```text
/releases/...
/snapshots/...
/private/...
```

Имена репозиториев не должны пересекаться со статическими маршрутами: `api`, `js`, `css`, `svg`, `assets`, `javadocs` и
т.д.

## Методы

| Метод      | Право  | Поведение                                                       |
|------------|--------|-----------------------------------------------------------------|
| GET        | чтение | Скачивание; HTML-запросы браузера могут упасть в management SPA |
| HEAD       | чтение | Только заголовки                                                |
| PUT / POST | запись | Загрузка / перезапись                                           |
| DELETE     | запись | Удаление; успех 204                                             |

Лимит тела около 2 GiB (`BodyLimit`); загрузки идут потоком.

### Загрузка

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Типичный успех: `201 Created`. Если redeploy выключен и файл есть, сервер отклоняет перезапись (любой не-2xx считайте
ошибкой).

Опциональный заголовок: `X-Generate-Checksums: true` пишет sidecar `.md5` / `.sha1` / `.sha256` / `.sha512`.

Сервер ведёт индекс, опциональные checksums и синхронизацию S3. Клиенты Maven видят обычную раскладку репозитория.

### Многочастная (chunked) загрузка — опционально

Оригинальный однозапросный `PUT` выше не меняется. Для больших файлов web UI может использовать параллельную
chunked-загрузку (с ретраями частей). Машинные клиенты могут использовать тот же API.

**Когда multi-part:** браузерный UI не дробит файлы меньше **8 MiB** (одиночный `PUT` быстрее). Машинные клиенты могут
открыть chunked-сессию для любого размера; сервер свернёт очень маленькие payload в одну часть.

Префикс: `/api/upload/chunked` (session cookie / Basic / Bearer; нужно право записи на целевой репозиторий).

Init и complete используют **`application/x-protobuf`** (`ChunkedUploadInitRequest` /
`ChunkedUploadInitResponse` / `ChunkedUploadCompleteResponse` в `proto/api/v1/api.proto`). Тела частей — сырой binary.

1. **`POST /api/upload/chunked/`** — начать сессию (`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`)

Логические поля (snake_case):

| Поле                 | Смысл                                                           |
|----------------------|-----------------------------------------------------------------|
| `purpose`            | `storage` (по умолчанию)                                        |
| `path`               | Назначение `repo/…/file`                                        |
| `filename`           | Опциональное отображаемое имя                                   |
| `size`               | Всего байт                                                      |
| `generate_checksums` | Писать sidecar checksums                                        |
| `chunk_size`         | Предпочтительный размер части (опционально; сервер нормализует) |

Поля ответа: `upload_id`, `chunk_size`, `chunk_count`, `purpose`. Всегда используйте возвращённые
`chunk_size` / `chunk_count` для последующих `PUT`.

**Правила размера частей** (сервер, `upload.NormalizeChunkSize`):

| Общий размер | Типичный размер части     |
|--------------|---------------------------|
| ≤ 256 KiB    | Одна часть = размер файла |
| ≤ 8 MiB      | Одна часть = размер файла |
| ≤ 32 MiB     | 4 MiB                     |
| ≤ 128 MiB    | 8 MiB                     |
| ≤ 512 MiB    | 16 MiB                    |
| ≤ 2 GiB      | 24 MiB                    |
| больше       | 32 MiB (макс.)            |

Клиентский `chunk_size` ограничен **256 KiB … 32 MiB**. Если частей > ~2048, сервер увеличивает размер части. Опустите
`chunk_size` (или `0`), чтобы принять таблицу выше.

2. **`PUT /api/upload/chunked/:upload_id/:index`** — сырое тело части (0-based), параллельно OK  
   Успех: `204`. Повтор PUT уже принятого index идемпотентен (безопасно для retry).

3. **`POST /api/upload/chunked/:upload_id/complete`** — собрать, индекс, опциональные checksums  
   Успех: `201` + `ChunkedUploadCompleteResponse` (`status=created`, `path=…`).

4. **`DELETE /api/upload/chunked/:upload_id`** — отмена и удаление temp (`204`).

Сессии истекают примерно через **15 минут** без complete (temps удаляются). Клиенты должны ретраить части с backoff.

### Скачивание

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC без auth. PRIVATE — Basic / Bearer.

При настроенных зеркалах отсутствующие локальные файлы могут подтягиваться с upstream (cache / negative-cache по конфигу
репозитория).

### Удаление

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## Доступ из браузера

С `Accept: text/html` отсутствующие репозитории или некоторые каталоги проваливаются в management SPA, чтобы
`http://host/releases/...` открывал UI. Машинным клиентам — `Accept: */*` или без Accept, чтобы избежать HTML.

## Превью Javadoc

Когда включено:

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

Нужно соответствующее право чтения. `raw` отдаёт файлы внутри jar. Размер ограничен `max_javadoc_size_mb`.

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

В `~/.m2/settings.xml` задайте username + password (или upload-токен) для server id.
