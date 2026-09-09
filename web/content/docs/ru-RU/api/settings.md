---
title: API настроек
order: 8
category: Справочник API
description: Доменные настройки сервиса, управление репозиториями и перестроение индекса
---

# API настроек

Маршруты требуют администратора или API Token с `admin:settings` либо `admin:repositories` в зависимости от операции.
Ответы используют protobuf, если это задано в `proto/api/v1/api.proto`.

## Получение доменов настроек

- **Путь**: `GET /api/settings/domains`
- **Ответ**: стабильные имена, поддерживаемые сервером, включая `server`, `proxy`, `storage`, `updater` и `index`.

## Чтение и изменение домена

- **Чтение**: `GET /api/settings/domain/:name`
- **Изменение**: `PUT /api/settings/domain/:name`
- **Поведение**: схема зависит от `:name`. Неизвестные поля и неверные значения отклоняются. Изменения хоста, порта,
  TLS, базы данных и некоторых параметров выполнения могут потребовать перезапуск.
- **GitHub OAuth**: `GET /api/settings/github-oauth` возвращает скрытое состояние, а
  `PUT /api/settings/github-oauth` обновляет Client ID и секрет, доступный только для записи.

**Другие провайдеры OAuth**: `GET /api/settings/oauth-providers` возвращает клиенты и шаблоны без секретов; `PUT /api/settings/oauth-providers` заменяет список. Массив `providers` обязателен; явно пустой массив удаляет все настроенные клиенты. Допускаются до 32 клиентов и тело до 128 KiB. Учётные данные, шаблоны и привязки описаны в разделе [Вход через сторонние сервисы](../security/oauth-login.md).

## Настройки репозиториев

Предпочтительны маршруты `/api/settings/repositories`. Maven-алиасы сохранены для совместимости.

### Список репозиториев

- **Путь**: `GET /api/settings/repositories`
- **Алиас**: `GET /api/settings/maven/repositories`

### Создание, изменение, удаление и миграция

- **Создать или изменить**: `PUT /api/settings/repositories/:name`
- **Удалить**: `DELETE /api/settings/repositories/:name`
- **Миграция Maven/files**: `POST /api/settings/repositories/:name/migrate/:target`, где `:target` — `maven` или
  `files`. Объекты не перемещаются; при возврате в Maven каталог перестраивается.

## Перестроение поискового индекса

- **Путь**: `POST /api/settings/index/rebuild`
- **Поведение**: отправляет объединяемую фоновую задачу и не запускает две перестройки параллельно.

## Срок резервирования доменов публикации

`GET /api/settings/maven-domains` и `PUT /api/settings/maven-domains` используют JSON.
Список настроек содержит `maven_domains`. Значение по умолчанию:

```json
{"release_value":2,"release_unit":"year"}
```

`release_value` — целое число от 1 до 100; `release_unit` принимает `month` или `year`, расчёт выполняется по календарю UTC.
Файл конфигурации хранит эти поля в `maven_domains`. Сохранённое изменение применяется к новым защитным блокировкам,
не меняя существующие даты освобождения или отдельный 31-дневный срок добровольного закрытия.
См. [состояние доменов Maven](maven.md).
