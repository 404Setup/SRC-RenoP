---
title: Maven
order: 4
category: API
---

# Просмотр Maven и вспомогательные API

Префикс: `/api/maven` (badge под `/api/badge`)

Эти эндпоинты читают индекс и метаданные. Байты артефактов лежат по `/{repo}/group/artifact/…` —
см. [storage.md](./storage.md).

Параметры пути используют раскладку Maven, например:

```text
com/example/demo
com/example/demo/1.0.0
```

Недостаточных прав чтения обычно даёт `404 Not found`.

## Детали каталогов и файлов (Protobuf)

### `GET /api/maven/details`

Репозитории, видимые текущему пользователю, как виртуальный корень.

Ответ: `FileDetails` (`application/x-protobuf`)

```text
type = DIRECTORY
name = "repositories"
files[] = { type: DIRECTORY, name: "<repo>" }
```

### `GET /api/maven/details/:repo_name`

Корень репозитория (с детьми).

### `GET /api/maven/details/:repo_name/*`

Детали пути. Каталоги включают `files`; файлы — `content_length` и `last_modified_time` (RFC3339Nano).

`type` — `FILE` или `DIRECTORY`.

### `GET /api/maven/repo-details/:repo_name`

Статистика и сводка зеркал. Ответ: `RepoDetailsResponse`.

| Поле                                                | Смысл                                                    |
|-----------------------------------------------------|----------------------------------------------------------|
| `name` / `visibility`                               | Имя, видимость                                           |
| `total_size` / `artifact_size` / `metadata_size`    | Байты                                                    |
| `total_files` / `artifact_count` / `metadata_count` | Счётчики (checksums и maven-metadata считаются metadata) |
| `mirrors[]`                                         | name, url, persist, cache_ttl, negative_cache, …         |

Нет доступа на чтение → **403** (в отличие от details, где часто 404).

## Запросы версий (JSON)

Путь должен указывать на координатный каталог с `maven-metadata.xml` (groupId/artifactId).

### `GET /api/maven/versions/:repo_name/*`

| Query    | По умолчанию | Смысл                     |
|----------|--------------|---------------------------|
| `filter` | —            | Подстрочный фильтр версий |
| `sorted` | `true`       | Сортировать результаты    |

```json
{
  "is_snapshot": false,
  "versions": ["1.0.0", "1.1.0"]
}
```

### `GET /api/maven/latest/version/:repo_name/*`

Те же query; добавьте `type=raw` для голой строки версии.

Иначе:

```json
{
  "is_snapshot": false,
  "version": "1.1.0"
}
```

### `GET /api/maven/latest/details/:repo_name/*`

`FileDetails` для последнего подходящего артефакта (**JSON**, не protobuf).

| Query        | По умолчанию | Смысл         |
|--------------|--------------|---------------|
| `extension`  | `jar`        | Расширение    |
| `classifier` | —            | Classifier    |
| `filter`     | —            | Фильтр версий |

### `GET /api/maven/latest/file/:repo_name/*`

Разрешить последнюю версию, затем получить через storage (редирект или тело — похоже на прямой URL артефакта).

## Badge

### `GET /api/badge/latest/:repo_name/*`

SVG-badge с последней версией. `Content-Type: image/svg+xml`.

| Query    | Смысл                                       |
|----------|---------------------------------------------|
| `name`   | Левая метка (по умолчанию: имя репозитория) |
| `color`  | Правый цвет (буквы/цифры или `#hex`)        |
| `prefix` | Префикс версии                              |
| `filter` | Фильтр версий                               |

```markdown
![latest](https://your-host/api/badge/latest/releases/com/example/demo)
```

## Генерация POM

### `POST /api/maven/generate/pom/:repo_name/*`

Требуется право записи в репозиторий.

```json
{
  "group_id": "com.example",
  "artifact_id": "demo",
  "version": "1.0.0"
}
```

Путь может уже оканчиваться на `.pom` или быть координатным каталогом (тогда имя файла — `artifact_id-version.pom`).

Недостаточно диска → 507. При успехе POM записывается и индекс обновляется.

## Политика конфиденциальности

### `GET|HEAD /api/privacy-policy`

Если в рабочей директории есть `privacy-policy.txt`, вернуть как `text/plain`; иначе 404. Не связано с Maven;
смонтировано в той же группе API.
