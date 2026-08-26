---
title: Безопасность и права
order: 1
category: Безопасность
description: Границы credentials, права репозитория, команды пакетов и defense in depth
---

# Безопасность и права

RenoP авторизует по типу credential, capability API Token, роли аккаунта, visibility репозитория и целевой команде.
Credential не сохраняет право после того, как его потерял владелец.

## Роли аккаунта и системы

| Роль или право | Эффект |
|:---------------|:-------|
| Anonymous | Чтение `PUBLIC` и известных exact paths в `HIDDEN` |
| `base` | Аутентифицированный аккаунт без неявной записи |
| `canview:{repo}` / `canview:*` | Чтение указанного или всех репозиториев, включая private |
| `canupdate:{repo}` / `canupdate:*` | Публикация с учётом package/domain policy |
| `showing` | Legacy compatibility; hidden остаётся вне пользовательских catalog |
| `allview` / `proview` | Legacy aliases глобального private read |
| `manager` / `admin` | Super-administrator пользователей, настроек, обновлений и всех команд |

System administrator глобален. L0-L4 остаются обычным уровнем collaboration. Операции администратора аудируются и не
создают молча отображаемого участника команды.

## Уровни репозитория и команды

- Visibility задаёт discovery/read boundary: `PUBLIC`, unlisted `HIDDEN` или authorized `PRIVATE`.
- Repository permission не создаёт Cargo/Docker package и не проверяет Maven domain автоматически.
- Cargo/Docker: L0 read, L1 publish, L2 lifecycle/metadata, L3 members, L4 ownership.
- Maven team привязан к проверенному global domain и действует во всех Maven repositories.
- Private Docker image не выдаёт public L0; blob ограничен image, доступным пользователю.

## Транспорт credentials

- **Browser session**: HttpOnly cookie `renop_session` для private security и Token management.
- **Basic**: имя плюс пароль/API Token, только стандартные пакетные протоколы.
- **Bearer API Token**: capability и exact-target policy для automation.
- **Docker Bearer**: краткосрочный token с actions, разрешёнными source credential и image.

`Authorization: Session`, session secret в URL и query credentials отклоняются. Scope/targets всегда пересекаются с
текущей авторизацией аккаунта.

## Defense in depth

- Пароли/recovery codes используют salted one-way verification; plaintext API Token не хранится.
- Sessions истекают и отзываются по устройству; recovery атомарно отзывает все существующие.
- Rate limits, progressive IP bans, active-request bounds и trusted proxy validation защищают сеть.
- Uploads, archives, mirrors и updates используют bounded streaming, path validation, hashes и temp storage.
- Audit и durable messages сохраняют результаты, не раскрывая оператора в neutral notifications.
