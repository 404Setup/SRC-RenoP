---
title: Установка
order: 2
category: Начало работы
description: Скачать бинарник RenoP
---

# Установка

## Скачать

Страница [Загрузки](/download):

- **Stable** — `https://mvnc.pkg.one/update/renop/stable/` (zip по платформам)
- **Preview** — `https://mvnc.pkg.one/update/renop/nightly/`

CI пишет `info.json` по каналам. Release notes — с GitHub.

Платформы как в build matrix: Windows, Linux, FreeBSD, NetBSD, OpenBSD; amd64/arm64 и доп. Linux-архи.

## Из zip

1. Скачать zip под OS/arch
2. Распаковать в рабочую директорию (конфиг рядом с CWD процесса)
3. `renop.exe` (Windows) или `./renop` (Unix)

По умолчанию слушает `0.0.0.0:3000`. Перед первым стартом задайте `RENOP_DEFAULT_ADMIN_PASSWORD` — [быстрый старт](./quickstart.md).

## Сборка из исходников

Нужны **Go**, **PowerShell 7**, **Node.js**.

```powershell
pwsh ./build.ps1                 # полная матрица → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64, Windows amd64
pwsh ./build.ps1 c               # текущая платформа
pwsh ./build.ps1 c nb            # текущая платформа, без zip
```

Подробности — в `README.md` репозитория.
