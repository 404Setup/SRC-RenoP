---
title: Установка и сборка
order: 2
category: Начало работы
description: Brotli-пакеты, CPU tiers, проверка и сборка из исходников
---

# Установка и сборка

## Готовые бинарные файлы

Скачайте raw Brotli package в [Центре загрузки](/download) или из официального канала:

- **Stable**: рекомендуется для production — `https://mvnc.pkg.one/update/renop/stable/`
- **Nightly**: ежедневные сборки с последними изменениями — `https://mvnc.pkg.one/update/renop/nightly/`

Новый формат — RFC 7932 `.br` stream. Центр загрузки может преобразовать его в legacy ZIP прямо в браузере.

## Уровни x86-64

| Уровень                         | Инструкции                      | Рекомендуемое применение                    |
|:--------------------------------|:--------------------------------|:--------------------------------------------|
| **x86-64-v1**                   | базовый x86-64                  | Старые серверы и generic VM                 |
| **x86-64-v2**                   | SSE3, SSSE3, SSE4.1/4.2, POPCNT | Обычные Intel/AMD с 2008 года               |
| **x86-64-v3** *(рекомендуется)* | AVX, AVX2, BMI1/2, FMA3         | Intel Haswell, AMD Zen 2 и новее            |
| **x86-64-v4**                   | основа AVX-512                  | Серверы с подтверждённой поддержкой AVX-512 |
| **ARM64**                       | NEON, Crypto                    | Apple Silicon, Graviton и ARM64 Linux       |

Выбирайте только поддерживаемый CPU tier. Бинарные v3/v4 не имеют динамического fallback на старый процессор.

## Проверка и запуск

`info.json` канала содержит SHA-256 каждой цели. Проверьте `.br` до распаковки:

```bash
# Linux
sha256sum -c SHA256SUMS --ignore-missing

# Windows (PowerShell)
Get-FileHash -Algorithm SHA256 .\renop-windows-amd64v3.br
```

Распакуйте stream в `renop` или `renop.exe`, при необходимости установите executable bit и запустите:

- **Linux / macOS**: `./renop`
- **Windows**: `.\renop.exe`

По умолчанию слушается `0.0.0.0:3000`. Начальный пароль описан в [Быстром старте](./quickstart.md).

## Регистрация системной службы

```bash
# Install and register as an auto-starting system service
./renop --install

# Stop and remove the system service
./renop --uninstall
```

Поддерживаются Windows SCM, systemd, OpenRC, LaunchDaemons и rc.d. См.
[Управление службой](../deployment/daemon.md).

## Сборка из исходников

Требования:

- **Go**: fork [404Setup/go](https://github.com/404Setup/go/releases), Go 1.28+
- **Frontend**: Node.js 18+ и pnpm
- **Сценарии**: PowerShell 7 (`pwsh`)
- **Protobuf**: `protoc` и `protoc-gen-go`

### Команды сборки

```powershell
# 1. Point GOROOT to 404Setup/go
$env:GOROOT = "D:\tools\go"
$env:PATH = "$env:GOROOT\bin;$env:PATH"

# 2. Install dependencies and compile frontend
pnpm install --frozen-lockfile
pnpm run build:frontend

# 3. Compile binary
pwsh ./build.ps1 c nb    # Current OS only, unzipped binary output
pwsh ./build.ps1 c       # Current OS packaged as a raw Brotli stream
pwsh ./build.ps1 s       # Mainstream platforms (Linux/Windows amd64/amd64v3/arm64)
pwsh ./build.ps1         # Full cross-compilation matrix
```

Сценарий автоматически устанавливает Brotli encoder CLI. Compilation использует до 4 задач; compression начинается
после готовности каждой цели и выполняется до 8 независимыми workers.
