---
title: Настройки
order: 6
category: API
---

# Настройки и конфигурация репозиториев

Префикс: `/api/settings`

Чтение и запись требуют **manager / admin**.

Все тела запросов/ответов под этим префиксом со структурированными данными используют **`application/x-protobuf`** (см.
`proto/api/v1/api.proto`). Пустые успешные тела остаются простым текстом (`""`). Ошибки валидации — короткий английский
текст.

Расположение на диске:

| Содержимое         | Файл                | Переменная окружения |
|--------------------|---------------------|----------------------|
| Доменные настройки | `config.yaml`       | `RENOP_CONFIG`       |
| Maven-репозитории  | `repositories.yaml` | `RENOP_REPOSITORIES` |

Изменения listener / TLS полностью применяются после перезапуска процесса.

## Индекс

### `POST /api/settings/index/rebuild`

Запрос: protobuf `RebuildIndexRequest`

| Поле   | Тип    | Значения         |
|--------|--------|------------------|
| `mode` | string | `full` \| `diff` |

| mode   | Поведение                                           |
|--------|-----------------------------------------------------|
| `full` | Асинхронная полная пересборка; очищает кэши Javadoc |
| `diff` | Дифференциальная пересборка                         |

Иное → 400 (`Invalid mode. Expected 'full' or 'diff'`). Успех: 200, пустая строка.

## Домены конфигурации

### `GET /api/settings/domains`

Ответ: protobuf `SettingsDomainsResponse`

| Поле      | Тип             |
|-----------|-----------------|
| `domains` | repeated string |

Типичные значения: `frontend`, `server`, `storage`, `updater`, `index`.

У `index` сейчас нет настраиваемых полей.

### `GET /api/settings/domain/:name`

Ответ: protobuf-сообщение домена (Content-Type `application/x-protobuf`).

**frontend** → `FrontendConfig`

| Поле                   | Тип    |
|------------------------|--------|
| `id`                   | string |
| `title`                | string |
| `description`          | string |
| `organization_website` | string |
| `organization_logo`    | string |
| `background_url`       | string |
| `icp_license`          | string |

**server** → `ServerConfig`

| Поле                  | Тип             |
|-----------------------|-----------------|
| `host`                | string          |
| `port`                | uint32          |
| `ssl_enabled`         | bool            |
| `ssl_cert_path`       | string          |
| `ssl_key_path`        | string          |
| `domains`             | repeated string |
| `enable_compression`  | bool            |
| `file_cache_size_mb`  | uint32          |
| `max_active_requests` | uint32          |
| `trusted_proxies`     | repeated string |
| `cdn_ip_header`       | string          |
| `cors_origins`        | repeated string |

**storage** → `StorageConfig`

| Поле                     | Тип    |
|--------------------------|--------|
| `storage_path`           | string |
| `enable_javadoc_preview` | bool   |
| `javadoc_extract_path`   | string |
| `max_javadoc_size_mb`    | int64  |

**updater** → `UpdaterConfig`

| Поле      | Тип    | Значения                                                     |
|-----------|--------|--------------------------------------------------------------|
| `channel` | string | `release` \| `nightly`                                       |
| `mode`    | string | `manual` \| `auto_check` \| `auto_install` \| `safe_install` |

**index** → пустой `IndexDomainSettings`

### `PUT /api/settings/domain/:name`

**Полная замена** домена. Тело — то же protobuf-сообщение, что GET для домена. Пропущенные поля Proto3 декодируются как
нули — клиенты должны отправлять полную конфигурацию домена (UI всегда шлёт полное состояние формы).

Успех: 200, пустая строка.

Правила:

- `frontend.background_url`: если не пусто — доступный, публичный IP, WebP, ≤ 5 MiB; частные адреса отклоняются
- `storage.max_javadoc_size_mb`: должен быть > 0
- `storage.storage_path`: при смене пути сервер сразу полностью пересобирает индекс файлов для нового корня (и
  перезапускает FS watcher); кэши Javadoc очищаются
- `updater.channel` / `updater.mode`: только допустимые enum-значения (пустые недопустимы)
- `index`: нечего писать → 404

Ошибка валидации → 400 + короткий английский текст.

## Maven-репозитории

### `GET /api/settings/maven/repositories`

Ответ: protobuf `MavenRepositoriesResponse` (`map<string, Repository>`).

| Поле                 | Смысл                                                     |
|----------------------|-----------------------------------------------------------|
| `name`               | Имя репозитория                                           |
| `visibility`         | `PUBLIC` / `HIDDEN` / `PRIVATE`                           |
| `allow_redeployment` | Можно ли перезаписывать существующие артефакты            |
| `mirrors[]`          | Upstream-зеркала (url, persist, TTL, auth, allow/deny, …) |
| `s3`                 | Опциональное S3-совместимое хранилище                     |

### `PUT /api/settings/maven/repositories/:name`

Создать или **полностью заменить**. Тело — protobuf `Repository`. Путь `:name` важнее body `name`.

Зарезервированные имена: `css`, `js`, `svg`, `api`, `javadocs`, `assets`, плюс недопустимые символы.

Успех: 200, пустая строка.

### `DELETE /api/settings/maven/repositories/:name`

Удалить из конфигурации; файлы на диске **не** удаляет. Успех: 200, пустая строка.
