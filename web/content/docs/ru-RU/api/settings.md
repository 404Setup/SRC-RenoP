---
title: API настроек
order: 8
category: Справочник API
description: Доменные настройки сервиса, управление репозиториями и перестроение индекса
---

# API настроек

Маршруты требуют администратора или API Token с `admin:settings` либо `admin:repositories` в зависимости от операции.
Ответы используют protobuf, если это задано в `proto/api/v1/api.proto`.

## 1. Получение доменов настроек

- **Путь**: `GET /api/settings/domains`
- **Ответ**: стабильные имена, поддерживаемые сервером, включая `server`, `proxy`, `storage`, `updater` и `index`.

## 2. Чтение и изменение домена

- **Чтение**: `GET /api/settings/domain/:name`
- **Изменение**: `PUT /api/settings/domain/:name`
- **Поведение**: схема зависит от `:name`. Неизвестные поля и неверные значения отклоняются. Изменения хоста, порта,
  TLS, базы данных и некоторых параметров выполнения могут потребовать перезапуск.
- **GitHub OAuth**: `GET /api/settings/github-oauth` возвращает скрытое состояние, а
  `PUT /api/settings/github-oauth` обновляет Client ID и секрет, доступный только для записи.

## 3. Настройки репозиториев

Предпочтительны маршруты `/api/settings/repositories`. Maven-алиасы сохранены для совместимости.

### Список репозиториев

- **Путь**: `GET /api/settings/repositories`
- **Алиас**: `GET /api/settings/maven/repositories`

### Создание, изменение, удаление и миграция

- **Создать или изменить**: `PUT /api/settings/repositories/:name`
- **Удалить**: `DELETE /api/settings/repositories/:name`
- **Миграция Maven/files**: `POST /api/settings/repositories/:name/migrate/:target`, где `:target` — `maven` или
  `files`. Объекты не перемещаются; при возврате в Maven каталог перестраивается.

## 4. Перестроение поискового индекса

- **Путь**: `POST /api/settings/index/rebuild`
- **Поведение**: отправляет объединяемую фоновую задачу и не запускает две перестройки параллельно.
