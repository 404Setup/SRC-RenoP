---
title: API аутентификации
order: 2
category: Справочник API
description: Браузерные сессии, профили, способы входа, восстановление и отзыв
---

# API аутентификации

Браузер использует HttpOnly cookie `renop_session`. Его секрет не возвращается API профиля и списка сессий и не
принимается в заголовках или URL. Закрытые настройки безопасности доступны только через браузерную сессию, не по
паролю или API Token.

## Вход по паролю или e-mail

- **Путь**: `POST /api/auth/login`
- **Аутентификация**: не требуется.
- **Тело**: protobuf `LoginRequest`; ниже показаны имена JSON. `name` принимает имя пользователя или закрытый e-mail.

### Запрос

```json
{
  "name": "admin",
  "secret": "your_password"
}
```

### Результат сессии

Успех устанавливает `renop_session` с `HttpOnly`, `SameSite=Lax` и `Secure` при HTTPS. Protobuf `SessionDetails`
содержит права и маршруты аккаунта, но оставляет `session_token` пустым.

## Вход Passkey и GitHub

- **Начало Passkey**: `POST /api/auth/fido/login/begin`
- **Завершение Passkey**: `POST /api/auth/fido/login/finish`
- **Начало GitHub**: `GET /api/auth/github/start`
- **Callback GitHub**: `GET /api/auth/github/callback`
- **Доступность GitHub**: `GET /api/auth/github/status`

GitHub отображается только после настройки OAuth администратором. RenoP запрашивает чтение пользователя и организаций,
сохраняет неизменяемые Provider ID и снимки principals, но не сохраняет OAuth Access Token.

## Текущий аккаунт и публичные профили

- **Текущая сессия**: `GET /api/auth/me`
- **Закрытый профиль**: `GET /api/auth/profile`
- **Изменить имя или псевдоним**: `PUT /api/auth/profile`
- **Изменить пароль**: `PUT /api/auth/profile/password`
- **Выйти**: `POST /api/auth/logout`
- **Публичный профиль**: `GET /api/users/:username/profile`
- **Участие в пакетах**: `GET /api/users/:username/memberships?format=cargo|docker|maven|npm`

Видимые маршруты используют имя, а неизменяемый ID остаётся внутренним. Участие в `HIDDEN` не возвращается; закрытые
связи видит только авторизованный пользователь.

## Безопасность аккаунта

Эти маршруты требуют текущую браузерную сессию и возвращают `Cache-Control: no-store`.

### E-mail и политика входа по паролю

- **Состояние**: `GET /api/auth/profile/security`
- **Задать e-mail**: `PUT /api/auth/profile/email`
- **Переключить вход по паролю**: `PUT /api/auth/profile/password-login`
- Отключить пароль можно только при наличии Passkey или GitHub. Для включения пароль должен быть задан.

### Коды восстановления

- **Создать**: `POST /api/auth/profile/recovery-codes`
- **Сбросить пароль**: `POST /api/auth/recovery/password`
- Двенадцать кодов показываются один раз; хранятся только verifier Argon2id. Четыре разных неиспользованных кода
  расходуются атомарно, сессии отзываются, а вход по паролю включается снова.

```json
{
  "identifier": "admin@example.com",
  "codes": ["CODE-ONE", "CODE-TWO", "CODE-THREE", "CODE-FOUR"],
  "new_password": "new_secure_password"
}
```

## Управление способами входа

- **Список Passkey**: `GET /api/auth/profile/fido`
- **Регистрация**: `POST /api/auth/profile/fido/register/begin`, затем
  `POST /api/auth/profile/fido/register/finish`
- **Удаление**: `DELETE /api/auth/profile/fido/:device_id`
- **Связанный GitHub**: `GET /api/auth/profile/github`
- **Отключить GitHub**: `DELETE /api/auth/profile/github`

Последний рабочий способ входа нельзя удалить или отключить.

## Браузерные сессии

- **Список**: `GET /api/auth/profile/sessions`
- **Отозвать одну**: `DELETE /api/auth/profile/sessions/:session_id`
- **Отозвать остальные**: `POST /api/auth/profile/sessions/revoke-others`

Список содержит публичный ID, способ входа, время, IP и User-Agent, но не секрет cookie.

## Окончательное закрытие учётной записи

`GET /api/auth/profile/retirement` возвращает условия закрытия текущей учётной записи. Этот маршрут и
`DELETE /api/auth/profile/retirement` требуют сеанса браузера. Запрос закрытия принимает JSON:

```json
{"confirmation":"alice"}
```

Подтверждение должно точно совпадать с именем пользователя. Успех возвращает `204 No Content`, несовпадение —
`400` и `ACCOUNT_RETIREMENT_CONFIRMATION`. Невыполненные условия возвращают `409`,
`ACCOUNT_RETIREMENT_BLOCKED` и актуальный план с полями `eligible`, `protected_role`,
`super_team_owner_count`, `maven_domain_owner_count`, `package_owner_count` и `pending_review_count`.

Системные администраторы и модераторы репозиториев не могут закрыть свои учётные записи. Не должно оставаться
ролей T4 глобальных команд, активных доменов Maven во владении, неустаревших пакетов с ролью L4 или ожидающих
проверки заявок. Сначала передайте владение, закройте домены либо окончательно пометьте пакеты устаревшими.

Закрытие навсегда резервирует учётную запись и имя, удаляет членство в командах, освобождает связь GitHub
и удаляет Passkey, сеансы, API Token, фото, коды восстановления и сообщения. Вход возвращает
`ACCOUNT_DELETED`. Личный адрес почты резервируется на 14 дней, журнал действий хранится 30 дней.
Плановая очистка обрабатывает истёкшие сроки ограниченными пакетами. Опубликованные пакеты остаются доступными для скачивания.

Освобождённую учётную запись GitHub можно сразу связать с другой активной учётной записью. Это не освобождает
зарезервированное имя, не сокращает срок удержания почты и не восстанавливает владение закрытыми ресурсами.

Администраторы получают сроки через `GET /api/tokens/:name/retention`, досрочно освобождают почту через
`DELETE /api/tokens/:name/retention/email` и очищают журнал через
`DELETE /api/tokens/:name/retention/audit`. Поля `deleted_at`, `email_release_at`,
`email_released_at`, `audit_purge_at` и `audit_purged_at` содержат миллисекунды Unix.
Административный маршрут `DELETE /api/tokens/:name` применяет те же условия окончательного закрытия.
