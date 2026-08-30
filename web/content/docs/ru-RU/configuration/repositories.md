---
title: Репозитории и зеркала
order: 2
category: Конфигурация
description: Движки, видимость, зеркала, миграция и S3
---

# Репозитории и зеркала

Определения находятся в `repositories.yaml`, путь можно заменить через `RENOP_REPOSITORIES`. Интерфейс администратора
изменяет те же проверяемые структуры. Имя — неизменяемый slug в нижнем регистре и первый сегмент URL.

## Пример конфигурации

```yaml
repositories:
  releases:
    name: releases
    format: maven
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: true
    publication_review: every_version
    download_statistics: true
    mirrors: []
  crates:
    name: crates
    format: cargo
    visibility: PUBLIC
    mirrors: []
  containers:
    name: containers
    format: docker
    visibility: PRIVATE
    allow_redeployment: false
    mirrors: []
```

## Поля репозитория

| Поле | По умолчанию | Описание |
|:-----|:-------------|:---------|
| `name` | Обязательно | Неизменяемый slug и префикс URL |
| `format` | `maven` | `maven`, `maven-classic`, `files`, `npm`, `cargo` или `docker` |
| `visibility` | `PUBLIC` | `PUBLIC`, `HIDDEN` или `PRIVATE` |
| `allow_redeployment` | `false` | Повтор Maven или замена files/Docker, если поддерживается |
| `require_gpg_signature` | `false` | Проверка отделённой OpenPGP подписи для Maven |
| `publication_review` | `off` | Режим проверки Maven/npm: `off`, `new_packages` или `every_version` |
| `download_statistics` | Зависит от движка | Включено для Maven/npm/Cargo/Docker; для `files` включается явно |
| `mirrors` | `[]` | Упорядоченные upstream definitions |
| `s3` | отсутствует | Отдельное S3-хранилище репозитория |

`maven-classic` меняет только компоновку интерфейса и сохраняет правила публикации Maven. `files` неструктурирован,
не создаёт контрольные суммы и POM и не проверяет подписи. Миграция Maven ↔ `files` не перемещает объекты; возврат в
Maven перестраивает каталог и восстанавливает сохранённую политику. Настройка статистики скачиваний также сохраняется.

Проверка публикаций поддерживает Maven и npm. Для Maven `allow_redeployment` принудительно становится `false`; npm
сохраняет транзакцию неизменяемой версии и dist-tag. Локальные файлы скрыты до одобрения модератором репозитория или
системным администратором, а зеркала не проверяются. Репозиторий с ожидающей задачей нельзя изменить, удалить или
перевести на другой движок.

Репозиторий `npm` требует резервирования до публикации, хранит неизменяемые SemVer и dist-tag, поддерживает scoped
private packages, команды L0-L4 и зеркала по точному имени или правилу `@scope/*`.

### Видимость

- **PUBLIC**: разрешены анонимное чтение и обнаружение.
- **HIDDEN**: не отображается в каталогах для анонимных пользователей и пользователей без прав, а также в профилях.
  Администраторы и пользователи с явным правом просмотра видят репозиторий; точный путь к файлу остаётся доступным.
- **PRIVATE**: чтение, список и запись требуют явного права. Закрытый Docker image также проверяет L0-L4.

## Upstream-зеркала

Отсутствующий object можно потоково получить с включённого зеркала и сохранить без полного buffer. Cargo и Docker
запрещают локальное создание, если подходящее имя существует upstream.

```yaml
mirrors:
  - name: "central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

| Поле | По умолчанию | Описание |
|:-----|:-------------|:---------|
| `name` | Обязательно | Уникальное имя в репозитории |
| `url` | Обязательно | Базовый upstream URL |
| `persist` | `true` | Сохранять успешные ответы |
| `cache_ttl_secs` | `86400` | Срок положительного cache |
| `negative_cache` | `true` | Cache поддерживаемых upstream miss |
| `timeout_secs` | `30` | Timeout одного запроса |
| `proxy` | `""` | Глобальный route, `direct` или именованный proxy |
| `allow_artifacts` | `[]` | Правила разрешения с учётом формата |
| `deny_artifacts` | `[]` | Приоритетные правила запрета |

Credentials задаются структурированными полями authorization и не встраиваются в `url`.

## S3-совместимое хранилище

Каждый репозиторий использует Disk или собственный S3. Repository gate сериализует смену storage/engine с uploads,
удалениями, GPG commit и mirror write.

```yaml
s3:
  enabled: true
  endpoint: "https://s3.us-east-1.amazonaws.com"
  bucket: "my-renop-bucket"
  key_prefix: "releases/"
  region: "us-east-1"
  access_key_id: "YOUR_ACCESS_KEY"
  secret_access_key: "YOUR_SECRET_KEY"
  force_path_style: false
  redirect_downloads: false
```

MinIO обычно требует `force_path_style`. При `redirect_downloads` RenoP авторизует запрос и отдаёт краткосрочный
подписанный redirect; иначе объект передаётся через RenoP.
