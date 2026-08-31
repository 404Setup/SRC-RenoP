---
title: Интеграция HTTP API
order: 19
category: Справочник API
description: Выбор API, protobuf media types, credentials, ошибки, повторы и совместимость клиентов
---

# Интеграция HTTP API

RenoP публикует management endpoints и пакетные протоколы на одной origin. Сначала выберите API family, затем media type
и credentials; считать все routes JSON REST API неправильно.

## Выбрать правильную поверхность API

| Поверхность             | Типичные пути                               | Клиент                                             |
|:------------------------|:--------------------------------------------|:---------------------------------------------------|
| Management/browser API  | `/api/...`                                  | RenoP UI, административные инструменты, automation |
| Maven и файлы           | `/{repo}/{path}`                            | Maven, Gradle, HTTP artifact clients               |
| Cargo sparse registry   | `/{repo}/config.json`, `/{repo}/api/v1/...` | Cargo и совместимые инструменты                    |
| npm registry            | `/{repo}/{package}`, `/{repo}/-/...`        | npm-compatible clients                             |
| Docker/OCI Distribution | `/v2/...`, `/v2/token`                      | Docker, Podman, OCI clients                        |
| Просмотр документации   | `/javadoc/...`, `/cargodoc/...`             | Browser после repository authorization             |

Не добавляйте `/api` к native package URL и не переносите методы/ошибки package protocol на management API.

## Использовать объявленное представление

Большинство management requests/responses используют `application/x-protobuf`. OpenAPI описывает logical fields, но его
примеры не означают поддержку JSON. Используйте messages из `proto/api/v1/api.proto` той же версии RenoP.

Для protobuf body явно задавайте:

```http
Content-Type: application/x-protobuf
Accept: application/x-protobuf
```

Health и некоторые ошибки — plain text. Cargo, npm и Docker/OCI используют JSON/binary своего протокола. Следуйте
документации endpoint, а не суффиксу пути.

## Выбрать credential по вызывающей стороне

| Credential             | Назначение                                     | Ограничение                                         |
|:-----------------------|:-----------------------------------------------|:----------------------------------------------------|
| Cookie `renop_session` | Interactive browser и private account security | HttpOnly; не извлекать для scripts                  |
| Bearer API token       | Management automation и поддерживаемые routes  | Доступ пересекается с текущими правами account/team |
| HTTP Basic             | Package clients и обозначенные upload flows    | Не является общей заменой session/Bearer            |
| Docker Bearer token    | Docker/OCI Distribution                        | Получается через challenge и token exchange         |

Token secret показывается при создании. Храните его в secret manager, задайте expiration, targets и минимальные scopes,
отзывайте после завершения работы. Query credentials и `Authorization: Session` отклоняются.

## Правильно построить базовый URL

В production используйте одну каноническую HTTPS-origin. Reverse proxy должен сохранить `Host` и scheme, чтобы cookies,
redirects, Docker challenges и generated URLs указывали на публичный сервис.

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

Успешное тело ответа — `"UP"`.

Health проверяет reachability, но не database/storage commit. Для deployment readiness добавьте отдельную
аутентифицированную операцию, если зависимости должны проверяться.

## Обрабатывать ответ в стабильном порядке

1. Прочитать HTTP status.
2. Проверить response `Content-Type`.
3. Прочитать `X-Renop-Error-Code`, если есть.
4. Декодировать body только соответствующим protocol decoder.
5. Записать timestamp и очищенный контекст, но не credential.

Management errors могут быть коротким текстом. Docker Distribution, Cargo и npm сохраняют structured errors. Не делайте
ветвление по полной английской фразе.

## Сопоставить статус и действие клиента

| Статус      | Действие                                                                                 |
|:------------|:-----------------------------------------------------------------------------------------|
| `200`–`204` | Декодировать документированный тип; успешное пустое тело может быть допустимо            |
| `202`       | Accepted, но ещё не обязательно visible; review может ожидать                            |
| `302`       | Следовать только для документированной загрузки, например authorized S3 presigned URL    |
| `400`       | Исправить request; автоматический retry обычно повторит ошибку                           |
| `401`       | Проверить допустимость credential type перед обновлением                                 |
| `403`       | Не повторять вслепую; изменить scopes, targets, permissions, team, policy или debug mode |
| `404`       | Проверить path/visibility; private/hidden data может скрываться намеренно                |
| `409`       | Перечитать state перед изменением immutable/concurrent operation                         |
| `413`       | Уменьшить payload только если это корректно, иначе исправить limits                      |
| `429`       | Соблюдать retry, добавить jitter, снизить concurrency                                    |
| `5xx`       | Повторять только bounded safe operations; сохранить error и проверить dependencies       |

## Повторять только при безопасной семантике

GET/HEAD обычно безопасны после transport failure. Для write определите idempotency и возможность commit до разрыва.
Используйте bounded exponential backoff с jitter и total deadline.

Не меняйте версию, не удаляйте данные и не расширяйте credentials ради повтора immutable publication. Chunked/registry
upload продолжайте по состоянию самого протокола.

## Соблюдать pagination и filters endpoint

У list endpoints нет единого cursor/page model. Используйте документированные параметры, сохраняйте stable IDs и
останавливайтесь по признаку окончания. UI filter не меняет authorization/visibility.

## Использовать контракты одной версии

Берите `web/assets/openapi.yaml` и `proto/api/v1/api.proto` из того же commit/release, что сервер. OpenAPI field
описывает
logical protobuf field, а не обязательно JSON. Maven, Cargo, npm и Docker должны использовать native configuration.

До production upgrade проверьте non-production: login, token authorization, repository list, read/write каждого формата,
pagination, error decoding и reverse proxy.

## Контрольный список интеграции

- [ ] Выбраны правильные API family и repository base path.
- [ ] HTTPS-origin, proxy host и scheme каноничны.
- [ ] Media types указаны явно.
- [ ] Credential type разрешён для route.
- [ ] Token scopes, targets, expiration и owner permission минимальны и актуальны.
- [ ] Status обрабатывается до текста body.
- [ ] Retry ограничен, содержит jitter и безопасен.
- [ ] Logs скрывают cookies, passwords, tokens и signed URLs.
- [ ] OpenAPI/protobuf соответствуют deployed release.
- [ ] Перед deployment выполняется native end-to-end test.

См. [API аутентификации](./authentication.md), [API-токены и пользователи](./tokens.md) и
[устранение неполадок](../guides/troubleshooting.md).
