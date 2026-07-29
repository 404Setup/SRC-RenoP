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

По умолчанию `0.0.0.0:3000`. Перед первым стартом задайте `RENOP_DEFAULT_ADMIN_PASSWORD` — [быстрый старт](./quickstart.md).

## Сборка из исходников

Нужны Go, PowerShell 7, Node.js.

```powershell
pwsh ./build.ps1                 # полная матрица → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64, Windows amd64
pwsh ./build.ps1 c               # текущая платформа
pwsh ./build.ps1 c nb            # текущая платформа, без zip
```

Подробности — в `README.md` репозитория.
