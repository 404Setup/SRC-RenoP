---
title: API реестра Cargo
order: 5
category: Справочник API
description: Sparse Index, публикация, скачивание и yank пакетов crate
---

# API реестра Cargo

RenoP реализует спецификации Cargo Registry и Sparse Index.

## Настройка Sparse Index (`config.json`)

- **Путь**: `GET /{repo}/config.json` или `GET /{repo}/index/config.json`
- **Назначение**: Cargo читает документ при первом подключении и узнаёт маршруты реестра.

### Ответ JSON

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## Метаданные Sparse Index

- **Путь**: `GET /{repo}/index/{prefix}/{crate_name}`
- **Назначение**: возвращает построчный JSON по стандартным правилам сегментации имён Cargo.

---

## Публикация crate

- **Путь**: `PUT /{repo}/api/v1/crates/new`
- **Аутентификация**: Token в `Authorization: <token>`.
- **Тело**: 4-байтовая длина JSON, метаданные JSON и бинарный архив `.crate`.
- **Конфликт имени**: первая публикация возвращает `409 Conflict`, если нормализованное имя занято локально или на
  применимом зеркале. Если проверка upstream не дала результата, возвращается `503 Service Unavailable`.

Для локальной публикации RenoP читает `package.readme` из проверенного `Cargo.toml` и извлекает указанный файл из
архива, не загружая crate целиком в память. Подробный ответ содержит не более 512 КиБ Markdown; браузер отображает его
через общий список разрешённых элементов и URL. Каталог и поиск не загружают содержимое README.

---

## Скачивание crate

- **Путь**: `GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **Ответ**: архив `.crate` с типом `application/x-tar`.

---

## Yank и unyank

- **Yank**: `DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank**: `PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **Аутентификация**: владелец crate или администратор.
