---
title: API Token и пользователи
order: 3
category: Справочник API
description: Жизненный цикл API Token, границы аутентификации и управление пользователями
---

# API Token и пользователи

API Token — долговременные машинные учётные данные одного аккаунта. RenoP хранит только поисковый SHA-256 digest
случайного 256-битного секрета. Открытое значение возвращается один раз при создании и не восстанавливается.

Каждый запрос проходит две независимые проверки:

- Token содержит способность, необходимую маршруту;
- аккаунт-владелец всё ещё может выполнить операцию над целевым ресурсом.

Изменения роли, прав репозитория и команды действуют без пересоздания Token.

## Управление своими API Token

Маршруты управления требуют HttpOnly cookie `renop_session`. API Token, пароль, `Authorization: Session` и параметры
URL не позволяют управлять секретами.

### Список доступных scopes

`GET /api/auth/profile/api-tokens/scopes`

Ответ фильтруется по текущим правам. Обычному аккаунту не предлагаются административные scopes.

```json
{
  "scopes": ["repository:read", "repository:publish", "package:metadata"],
  "target_kinds": {
    "repository:read": "repository",
    "repository:publish": "repository",
    "package:metadata": "package"
  },
  "target_limit": 128
}
```

### Создание Token

`POST /api/auth/profile/api-tokens`

```json
{
  "name": "CI publishing",
  "scopes": ["repository:read", "repository:publish"],
  "targets": {
    "repository:publish": ["releases"]
  },
  "expires_at": 1798761600000
}
```

`expires_at` — необязательное время Unix в миллисекундах от пяти минут до пяти лет после создания. Отсутствие или null
означает отсутствие срока Token. Один аккаунт может иметь до 50 API Token.

`targets` отдельно ограничивает каждый scope. Scope, не указанный в `targets`, действует для всех целей, которые сейчас
разрешены аккаунту. Цель репозитория — точное имя; цель пакета — `repository/package`. Для Maven используйте, например,
`maven-releases/com.example/library`. Цель команды — `package/repository/package` или `domain/example.com`, цель
домена —
каноническое имя. Общий предел — 128 целей.

Ограничения целей не обходят права репозитория и текущие уровни L0-L4.

Успех возвращает `201 Created` и `Cache-Control: no-store`:

```json
{
  "token": {
    "id": "07cdcf2e-0828-4a29-9817-cf771cc9fb0a",
    "name": "CI publishing",
    "scopes": ["repository:publish", "repository:read"],
    "targets": {"repository:publish": ["releases"]},
    "created_at": 1787731200000,
    "expires_at": 1798761600000
  },
  "secret": "rnp_pat_EXAMPLE_REDACTED_COPY_THE_REAL_VALUE_ONCE"
}
```

### Список метаданных Token

`GET /api/auth/profile/api-tokens` возвращает только несекретные метаданные и предел аккаунта.

### Отзыв Token

`DELETE /api/auth/profile/api-tokens/{token_id}` возвращает `204 No Content` и немедленно очищает кеш аутентификации.

## Справочник scopes

| Scope                 | Возможность                                                                       |
|:----------------------|:----------------------------------------------------------------------------------|
| `repository:read`     | Чтение каталогов, метаданных, файлов, образов и версий                            |
| `repository:publish`  | Публикация через Maven, npm, Cargo, Docker, files или блочную загрузку            |
| `repository:delete`   | Удаление файлов, версий, тегов и образов                                          |
| `package:create`      | Резервирование npm/Cargo package или Docker image после проверки репозитория      |
| `package:metadata`    | Изменение описания и метаданных пакета                                            |
| `package:lifecycle`   | Archive, restore, yank и unyank пакета или версии                                 |
| `team:manage`         | Просмотр и управление командами и приглашениями npm, Cargo, Docker и Maven domain |
| `domain:read`         | Чтение закрытой конфигурации Maven domain                                         |
| `domain:create`       | Создание Maven domain                                                             |
| `domain:verify`       | Проверка или принудительная проверка Maven domain                                 |
| `domain:delete`       | Удаление Maven domain                                                             |
| `messages:read`       | Чтение, отметка и удаление сообщений аккаунта                                     |
| `account:read`        | Чтение закрытых данных аккаунта и личного аудита                                  |
| `account:write`       | Изменение публичного профиля через API                                            |
| `statistics:read`     | Запрос доступной аккаунту статистики скачиваний                                   |
| `admin:users`         | Управление аккаунтами и устройствами входа                                        |
| `admin:repositories`  | Управление репозиториями и перестроение индексов                                  |
| `admin:settings`      | Управление системными настройками и диагностикой                                  |
| `admin:audit`         | Чтение и очистка административного аудита и состояния                             |
| `admin:notifications` | Создание уведомлений администратора                                               |
| `admin:updates`       | Проверка, загрузка, установка и перезапуск обновлений                             |
| `admin:statistics`    | Запрос системной статистики                                                       |

`admin:*` может создать только администратор; scope перестаёт действовать, если владелец теряет эту роль. Старые
`package:manage` и `domain:manage` принимаются для существующих Token, но больше не назначаются.

## Использование Token

Для разрешённого API управления используйте Bearer:

```http
Authorization: Bearer rnp_pat_REDACTED
```

Пакетный клиент может использовать Token как Basic password с именем владельца. Basic ограничен пакетными протоколами.
npm передаёт Token через `_authToken` или Basic; Cargo — целиком в `Authorization`. Docker обменивает его через
`/v2/token`; краткосрочный Token содержит
только операции, разрешённые одновременно scopes и правами образа.

## Совместимость

CRUD пользователей администратора остаётся в `/api/tokens`, но администратор не может создать учётные данные другому
пользователю. Старый `POST /api/auth/profile/token` создаёт дополнительный бессрочный Token публикации для текущего
аккаунта. Новые интеграции должны применять подробные маршруты профиля.
