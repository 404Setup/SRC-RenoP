---
title: Установка
order: 2
category: Начало работы
description: Скачать бинарник RenoP
---

# Установка

## Скачать

[Страница загрузок](/download) или zip напрямую:

- Stable: `https://mvnc.pkg.one/update/renop/stable/`
- Preview: `https://mvnc.pkg.one/update/renop/nightly/`

## Из zip

1. Распаковать в рабочую директорию
2. `renop.exe` (Windows) или `./renop` (Unix)

По умолчанию `0.0.0.0:3000`. Перед первым стартом задайте
`RENOP_DEFAULT_ADMIN_PASSWORD` — [быстрый старт](./quickstart.md).

## Сборка из исходников

Используйте [наш fork Go](https://github.com/404Setup/go/releases), а не официальную сборку. Также нужны PowerShell 7 и
Node.js.

1. Посмотрите версию `go` в `go.mod`.
2. Скачайте последний release `go<версия>` для своей ОС и архитектуры.
3. Проверьте архив по файлу `SHA256SUMS` из того же release.
4. Распакуйте архив, задайте `GOROOT` равным каталогу `go`, добавьте `GOROOT/bin` в `PATH` и запустите `go version`.

```powershell
pwsh ./build.ps1
pwsh ./build.ps1 s
pwsh ./build.ps1 c
pwsh ./build.ps1 c nb
```

Подробности — в `README.md` репозитория.
