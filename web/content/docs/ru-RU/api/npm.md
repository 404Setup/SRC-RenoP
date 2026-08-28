---
title: API реестра npm
order: 7
category: Справочник API
description: Метаданные npm, публикация, tarball, dist-tag, команды и административные маршруты
---

# API реестра npm

Каждый репозиторий формата `npm` предоставляет совместимый JSON-реестр под `/{repo}/`. До первой публикации имя пакета
резервируется через административный API или веб-интерфейс.

## Обнаружение реестра и аккаунт

- **Доступность**: `GET /{repo}/-/ping`
- **Текущий аккаунт**: `GET /{repo}/-/whoami`
- **Поиск**: `GET /{repo}/-/v1/search?text={query}&size={limit}&from={offset}`

Ошибки протокола представлены JSON со стабильными полями `error` и `reason`:

```json
{
  "error": "not_found",
  "reason": "npm package was not found"
}
```

## Метаданные пакета и tarball

- **Полный или сокращённый packument**: `GET /{repo}/{package}`
- **Tarball**: `GET /{repo}/{package}/-/{name}-{version}.tgz`
- **Публикация или правка метаданных**: `PUT /{repo}/{package}`

Имя со scope можно кодировать одним параметром, например `%40example%2Flibrary`. Packument поддерживает ETag и
Last-Modified. Запрос `application/vnd.npm.install-v1+json` получает ограниченные сокращённые метаданные. Для приватных
ответов общий кэш запрещён.

Документ публикации содержит одну SemVer-версию и одно base64-вложение tarball. JSON ограничен 96 MiB, сжатый tarball
64 MiB, распакованные данные 512 MiB, число файлов 100,000, `package.json` 2 MiB. Пакет хранит не более 5,000 версий
и 4 MiB совокупных метаданных версий. Сервер потоково пишет декодированный архив во временное хранилище и не публикует
частично проверенный tarball.

## Dist-tag и жизненный цикл

- **Список тегов**: `GET /{repo}/-/package/{package}/dist-tags`
- **Установка тега**: `PUT /{repo}/-/package/{package}/dist-tags/{tag}`
- **Удаление тега**: `DELETE /{repo}/-/package/{package}/dist-tags/{tag}`
- **Обновление или снятие с revision**: `PUT /{repo}/{package}/-rev/{revision}`
- **Удаление пакета с revision**: `DELETE /{repo}/{package}/-rev/{revision}`

Версии неизменяемы. Снятие и удаление создают tombstone, поэтому номер опубликованной версии нельзя использовать снова.
Конфликт revision возвращает `409 Conflict` и требует повторно получить текущий packument.

## Административный API браузера

Маршруты того же origin используют JSON и при ошибке возвращают стабильный заголовок `X-Renop-Error-Code`.

- `GET /api/npm/repositories/{repo}/packages`
- `POST /api/npm/repositories/{repo}/packages`
- `PUT /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/versions?package={package}&version={version}`
- `GET /api/npm/repositories/{repo}/owners?package={package}`
- `POST /api/npm/repositories/{repo}/owners?package={package}`
- `PUT /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `DELETE /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `GET /api/npm/repositories/{repo}/users/search?package={package}&q={query}`
- `POST /api/npm/repositories/{repo}/invitations/{id}/{accept|reject}`

Каталог разбит на страницы с `limit` от 1 до 100 и ограниченным `offset`. Приватные пакеты скрыты без участия или
административного доступа. Команда доступна только участникам L3/L4 и администраторам.

## Аутентификация и авторизация

Клиент npm использует Basic с паролем или API Token либо API Token как `_authToken`. Bearer scope пересекаются с
текущими правами аккаунта и точными целями репозитория, пакета или команды. Публикация требует существующего пакета и
L1, метаданные и снятие L2, управление командой L3, владение и удаление пакета L4.
