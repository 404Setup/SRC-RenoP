---
title: API Docker / OCI Registry v2
order: 6
category: Справочник API
description: Маршруты OCI Distribution v2 и Docker Registry v2
---

# API Docker / OCI Registry v2

RenoP реализует OCI Distribution Spec v2 и Docker Registry v2.

Образ контейнера является явным ресурсом. Перед запросом push-прав создайте его через
`POST /api/docker/repositories/:repo/images` или на странице репозитория. Маршруты blob и manifest не создают образ
автоматически. Закрытый образ не попадает в неавторизованные каталоги; для его manifest и связанных blob требуется
участник L0-L4 или администратор.

Создание возвращает `409 Conflict`, если нормализованное имя занято локально или на подходящем включённом зеркале. Если
проверка upstream не дала результата, имя не резервируется и возвращается `503 Service Unavailable`.

API управления возвращает читаемое тело и `X-Renop-Error-Code`; интерфейс переводит код, не показывая сырой текст.
Маршруты OCI используют предписанную спецификацией структуру `errors`.

## Проверка версии

- **Путь**: `GET /v2/` или `HEAD /v2/`
- **Ответ**:
    - `200 OK` с `Docker-Distribution-API-Version: registry/2.0`;
    - при необходимости входа — `401 Unauthorized` с
      `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"`.

---

## Аутентификация Bearer Token

- **Путь**: `GET /v2/token` или `GET /v2/auth`
- **Назначение**: обмен Basic Auth на временный Docker Token. API Token требует `repository:read` для pull,
  `repository:publish` для push и `repository:delete` для удаления. Видимость и уровень L0-L4 проверяются отдельно
  перед выдачей каждого действия.

---

## Каталог и теги

### Список образов

- **Путь**: `GET /v2/_catalog`
- **JSON**: `{"repositories": ["my-org/my-app"]}`

### Список тегов

- **Путь**: `GET /v2/:name/tags/list`
- **JSON**: `{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## Операции manifest

- **Чтение**: `GET /v2/:name/manifests/:reference`
- **Публикация**: `PUT /v2/:name/manifests/:reference` (образ создан, уровень не ниже L1)
- **Удаление**: `DELETE /v2/:name/manifests/:reference`

---

## Операции blob

- **Проверка**: `HEAD /v2/:name/blobs/:digest`
- **Скачивание**: `GET /v2/:name/blobs/:digest`
- **Начало**: `POST /v2/:name/blobs/uploads/` (поддерживается `?mount=<digest>&from=<other_repo>`)
- **Добавление блока**: `PATCH /v2/:name/blobs/uploads/:uuid`
- **Завершение**: `PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
