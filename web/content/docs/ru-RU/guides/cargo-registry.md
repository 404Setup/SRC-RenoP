---
title: Реестр Cargo (Rust)
order: 2
category: Руководства
description: Создание Cargo repository, Sparse Index, публикация, ownership и Cargodoc
---

# Руководство по Cargo (Rust) Registry

До настройки клиента создайте repository формата `cargo`. В примерах используется `crates`. RenoP реализует Cargo
Sparse Index и потоково отдаёт архивы без клонирования Git index.

## Настройка Cargo (`.cargo/config.toml`)

```toml
[registries.renop]
index = "sparse+http://localhost:3000/crates/"

# Optional: replace default crates.io upstream
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/crates/"
```

В production используйте HTTPS. `config.json` сообщает download и API routes. Private repository включает
`auth-required` и требует credentials для index и crates.

## Аутентификация

Создайте отдельный истекающий API Token. Для первой публикации обычно нужны `repository:read`, `repository:publish` и
`package:create`. Для archive/yank добавьте `package:lifecycle`, для owner management — `team:manage`.

```bash
cargo login --registry renop
# Paste your RenoP token when prompted
```

Cargo сохраняет значение в `~/.cargo/credentials.toml`:

```toml
[registries.renop]
token = "your_renop_token"
```

Token отправляется как полное значение `Authorization`. RenoP пересекает scope/targets с текущими правами аккаунта,
репозитория и package team.

## Зависимости и публикация

### Добавление зависимости (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### Публикация crate

```bash
cargo publish --registry renop
```

Первая успешная публикация резервирует нормализованное имя и выдаёт издателю L4. Локальное имя или имя на подходящем
зеркале отклоняется. Неопределённая upstream check безопасно возвращает `503` без резервирования. Следующие версии
требуют уровень публикации команды.

Если проверка публикаций включена, после сохранения архива `cargo publish` возвращает `202 Accepted`. До одобрения
модератором репозитория или системным администратором crate отсутствует в sparse index и публичном каталоге. В режиме
`new_packages` это правило действует до появления первой видимой версии. Зеркальные crates проверку не проходят.

### Поиск, yank и unyank

```bash
# Search crates
cargo search --registry renop my-crate

# Yank a version
cargo yank --registry renop --version 0.1.0 my-crate

# Unyank
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

Владелец управляет L0-L4 участниками и приглашениями на странице пакета. Mirror crates отмечены как upstream, не имеют
локального владельца и остаются read-only.

## Cargodoc

RenoP проверяет и извлекает rustdoc в sandboxed viewer. Включите Cargodoc и size limits в `config.yaml`.

URL: `http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
