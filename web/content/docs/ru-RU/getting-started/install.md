---
title: Установка
order: 2
category: Начало работы
description: Скачать и разместить бинарник RenoP
---

# Установка

## Официальные загрузки

Используйте страницу [Загрузки](/download) на сайте:

- **Стабильная** — assets GitHub Release для каждой ОС/архитектуры
- **Preview** — CI-артефакт `dist-artifacts` из workflow `build.yml` через [nightly.link](https://nightly.link), с
  извлечением пакета платформы в браузере

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
