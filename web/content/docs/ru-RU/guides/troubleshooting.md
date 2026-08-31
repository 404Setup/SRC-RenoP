---
title: Устранение неполадок
order: 5
category: Руководства
description: Диагностика запуска, аутентификации, прокси, протоколов, зеркал и хранилищ от HTTP-статуса
---

# Устранение неполадок

Начните с HTTP-статуса, точного URL, format/visibility репозитория и типа credentials. Не меняйте несколько параметров
одновременно: пакетный клиент может скрыть исходный ответ общим сообщением.

## Собрать минимальные данные

До перезапуска или удаления состояния зафиксируйте:

- версию и время запуска RenoP;
- время, method, очищенный URL и response status;
- repository name, format, visibility, наличие mirror/review;
- client/version, command и verbose log без секретов;
- строки server log и `X-Renop-Error-Code`, если он есть;
- доступность базы/storage, свободное место и последние изменения.

Не публикуйте session cookies, token secrets, passwords, S3/OAuth keys и полные Authorization headers.

## Процесс не запускается

Сначала проверьте пути и working directory. Относительные пути `config.yaml`, `repositories.yaml`, SQLite, `index.json`
и local storage разрешаются в окружении сервиса, которое может отличаться от shell.

Частые причины: занятый порт, неверный YAML, недоступный DSN, отсутствие write permission, некорректные TLS files или
нет доступа к secrets. `RENOP_DEFAULT_ADMIN_PASSWORD` создаёт первый account, но не сбрасывает существующего admin.

## Health работает, приложение — нет

```bash
curl -i https://packages.example.com/api/status/health
```

`"UP"` подтверждает только ответ health route. Он не тестирует login, database writes, local/S3 storage, mirrors или
publication policy. Выполните аутентифицированный запрос и операцию с одноразовым пакетом.

Если UI сообщает о новой версии интерфейса, перезагрузите страницу до диагностики protobuf/route. Proxy или browser
cache
может отдавать JavaScript другой версии.

## Сначала читать статус, затем текст

| Статус | Первые проверки                                                                                       |
|:-------|:------------------------------------------------------------------------------------------------------|
| `400`  | Неверный protobuf/JSON, path/name, обязательное поле или неподдерживаемая операция                    |
| `401`  | Credential отсутствует, истёк, некорректен или запрещён; cookie не возвращается через HTTPS/proxy     |
| `403`  | Account permission, token scope/target, team level, visibility, debug mode или review role            |
| `404`  | Неверный repository/path, hidden resource, отсутствующая версия, mirror miss или скрытая private data |
| `409`  | Immutable version/tag, existing reservation, state transition или concurrent decision                 |
| `413`  | Upload limit proxy/server; проверить размер и buffering                                               |
| `429`  | Rate/concurrency control; соблюдать retry и снизить parallelism                                       |
| `5xx`  | Database, storage, upstream, signing, extraction или internal failure; смотреть logs                  |

Plain-text фразы предназначены людям и могут меняться. Используйте status, protocol-native body и стабильный header.

## Аутентификация и браузерные сессии

UI использует HttpOnly cookie `renop_session`. Private security endpoints не принимают password, Bearer token,
`Authorization: Session` или session в URL. Проверьте HTTPS, исходные scheme/host через proxy и возврат cookie той же
origin.

Для automation используйте scoped Bearer. Доступ пересекается со scopes, targets, account permission, repository policy
и package-team membership. Более широкий token не заменяет отсутствующее право account/team.

## Maven и Gradle

- URL заканчивается именем репозитория RenoP, а не `/api`.
- Maven `<server><id>` совпадает с ID в `distributionManagement` или dependency repository.
- Basic username — имя account, password — API token с необходимыми scopes.
- `groupId` находится под контролируемым publishing domain и имеет нужный team level.
- Для signed repository загружена detached signature и проверена backend record, а не только filename.
- Immutable release redeploy должен отклоняться; не обходите это удалением server files.

## Cargo

- Используйте sparse URL с repository path и завершающим `/`: `sparse+https://packages.example.com/crates/`.
- Выполните `cargo login --registry <name>` и сохраните полное значение RenoP token.
- Различайте `repository:publish`, `package:create`, lifecycle и team-management scopes.
- Если upstream name check недоступен, первая публикация безопасно отклоняется без reservation; повторите после
  восстановления.
- Пока review pending, archive не виден в sparse index и public catalog.

## npm

- Укажите registry с путём репозитория, не только host; при необходимости настройте scoped registry.
- Проверьте token entry в user/CI `.npmrc` и не коммитьте его.
- Reserve package перед первой публикацией, если этого требует policy.
- Version immutable; увеличение concurrency не исправит conflict.
- Для mirror package отличайте upstream version от local ownership до изменения teams/dist-tags.

## Docker и OCI

- Login выполняется к registry host; image name/path передаётся отдельно в `pull`, `push` или Podman.
- Используйте доверенный certificate; insecure registry допустим только в изолированном тесте.
- Create/reserve image или namespace до первого push, если требует policy.
- Proxy сохраняет `/v2/` challenge и `/v2/token`; удаление `Authorization` или rewrite path ломает Bearer flow.
- При push failure определите blob, manifest или tag и сравните digest/media type.

## Зеркала, хранилище и обратный прокси

Mirror miss может означать upstream `404`, negative cache, allowlist denial, expired credential, outbound proxy или
local
commit failure. Сравните direct upstream с запросом через RenoP, не обходя production authorization.

Для S3 проверьте endpoint, region, path style, bucket, prefix, time, TLS и read/write/list/delete. Presigned URL
тестируйте
из клиентской сети. Для local storage проверьте ownership, free space, temporary capacity и atomic rename.

Для крупных загрузок отключите buffering/body limit и увеличьте timeouts. Forwarded headers доверяйте только configured
proxies.

## Эскалировать с воспроизводимым примером

Сведите ошибку к одному repository, disposable package и command. Приложите очищенную configuration, expected/actual
status,
результат без reverse proxy и уже выполненные действия. Не удаляйте database, storage prefix или ownership до сбора
доказательств.

См. [Интеграция HTTP API](../api/client-integration.md) и
[проверка перед промышленным запуском](../deployment/production-checklist.md).
