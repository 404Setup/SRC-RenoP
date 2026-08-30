---
title: Управление системной службой
order: 1
category: Развёртывание
description: Регистрация RenoP как нативной службы через --install и --uninstall
---

# Управление системной службой

RenoP может зарегистрироваться как автоматически запускаемая служба ОС без сторонних wrappers.

## Команды

```bash
# Register and start as a system service
./renop --install

# Configure a local Caddy reverse proxy
./renop --install-caddy --hostname renop.example.com

# Stop and remove the system service
./renop --uninstall

# View CLI help
./renop --help
```

`--install` сохраняет абсолютный путь бинарного файла и использует его каталог как рабочий. Запускайте команду из
окончательного места, например `/opt/renop` или `C:\Program Files\RenoP`.

## Поддерживаемые платформы

| ОС                  | Менеджер | Поведение                                                        |
|:--------------------|:---------|:-----------------------------------------------------------------|
| **Windows**         | SCM      | Служба `RenoP` с автозапуском, доступная в `services.msc`        |
| **Linux (systemd)** | systemd  | Создаёт `/etc/systemd/system/renop.service` и включает службу    |
| **Linux (OpenRC)**  | OpenRC   | Создаёт `/etc/init.d/renop` и добавляет в default runlevel       |
| **macOS**           | launchd  | Создаёт и загружает `/Library/LaunchDaemons/one.pkg.renop.plist` |
| **BSD**             | rc.d     | Генерирует подходящий сценарий rc.d                              |

Установка и удаление требуют системных привилегий. Перемещайте или заменяйте бинарный файл только штатным механизмом
обновления, чтобы сохранить зарегистрированный путь.

## Обычные операции

### Linux (systemd)

```bash
systemctl status renop    # Check service status
systemctl restart renop   # Restart service
journalctl -u renop -f    # Tail real-time logs
```

### Windows (PowerShell)

```powershell
Get-Service RenoP         # Check service status
Restart-Service RenoP     # Restart service
```
