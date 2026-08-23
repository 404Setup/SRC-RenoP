---
title: System Service Management
order: 1
category: Deployment
description: Registering RenoP as a native system service using --install and --uninstall
---

# System Service Management

RenoP includes native operating system service management commands, allowing it to be registered as an auto-starting
background service without third-party wrappers.

## 1. Commands

```bash
# Register and start as a system service
./renop --install

# Stop and remove the system service
./renop --uninstall

# View CLI help
./renop --help
```

Running `--install` records the absolute path to the binary and sets its directory as the working directory. Execute
this command inside your permanent deployment directory (e.g. `/opt/renop` or `C:\Program Files\RenoP`).

## 2. Platform Support

| Operating System          | Service Manager       | Details                                                                                 |
|:--------------------------|:----------------------|:----------------------------------------------------------------------------------------|
| **Windows**               | Windows Service (SCM) | Registers as `RenoP` service with Automatic start type; manageable via `services.msc`   |
| **Linux (systemd)**       | systemd               | Creates `/etc/systemd/system/renop.service` and executes `systemctl enable --now renop` |
| **Linux (OpenRC)**        | OpenRC                | Creates `/etc/init.d/renop` and executes `rc-update add renop default`                  |
| **macOS**                 | launchd               | Creates and loads `/Library/LaunchDaemons/one.pkg.renop.plist`                          |
| **BSD (FreeBSD/OpenBSD)** | rc.d                  | Generates rc.d service scripts                                                          |

## 3. Service Operations

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
