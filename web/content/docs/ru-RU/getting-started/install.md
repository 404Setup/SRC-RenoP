---
title: Установка
order: 2
category: Начало работы
description: Скачать и разместить бинарник RenoP
---

# Установка

## Официальные загрузки

Используйте страницу [Загрузки](/download) на сайте:

- **Стабильная** — официальный источник `https://mvnc.pkg.one/update/renop/stable/` (zip по платформам)
- **Preview** — официальный источник `https://mvnc.pkg.one/update/renop/nightly/` (zip по платформам)

Метаданные публикуются CI в `info.json`. Changelog по-прежнему берётся с GitHub.

Поддерживаемые платформы соответствуют матрице сборки проекта (Windows, Linux, FreeBSD, NetBSD, OpenBSD; amd64/arm64 и
дополнительные Linux-архитектуры).

## Из архива релиза

1. Скачайте zip для своей платформы
2. Распакуйте
3. Запустите `renop.exe` в Windows или `./renop` на Unix-подобных системах

По умолчанию RenoP слушает `0.0.0.0:3000`.

## Сборка из исходников

Нужны **Go**, **PowerShell 7** и **Node.js** (Rolldown-сборка фронтенда).

```powershell
pwsh ./build.ps1                 # полная матрица, пакеты в dist/
pwsh ./build.ps1 s               # Linux amd64/arm64 и Windows amd64
pwsh ./build.ps1 c               # только текущая платформа
pwsh ./build.ps1 c nb            # текущая платформа, без архива
```

Подробности по protobuf и фронтенду — в `README.md` репозитория.
