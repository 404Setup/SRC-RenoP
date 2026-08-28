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
