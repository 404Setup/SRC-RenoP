---
title: 系统服务管理
order: 1
category: 运维部署
description: 使用 --install 与 --uninstall 注册系统守护进程与开机自启
---

# 系统服务管理

RenoP 二进制内置了对主流操作系统的服务管理能力，可以通过命令行直接将其注册为后台常驻的系统服务，无需编写额外的配置文件。

## 常用命令

```bash
# Register and start as a system service
./renop --install

# Stop and remove the system service
./renop --uninstall

# View CLI help
./renop --help
```

执行 `--install` 时，程序会将可执行文件的当前绝对路径以及所在目录作为服务的工作目录进行登记。因此，建议在固定的安装目录（如
`/opt/renop` 或 `C:\Program Files\RenoP`）下执行该命令。

## 各操作系统平台支持

| 操作系统                  | 服务管理器            | 安装与管理机制                                                                                 |
|:--------------------------|:----------------------|:-----------------------------------------------------------------------------------------------|
| **Windows**               | Windows Service (SCM) | 注册为名为 `RenoP` 的 Windows 服务，自启动类型设为自动，可通过 `services.msc` 或 `sc.exe` 管理 |
| **Linux (systemd)**       | systemd               | 生成 `/etc/systemd/system/renop.service` 单元文件并执行 `systemctl enable --now renop`         |
| **Linux (OpenRC)**        | OpenRC                | 生成 `/etc/init.d/renop` 启动脚本并执行 `rc-update add renop default`                          |
| **macOS**                 | launchd               | 生成 `/Library/LaunchDaemons/one.pkg.renop.plist` 并加载运行                                   |
| **BSD (FreeBSD/OpenBSD)** | rc.d                  | 生成 `/etc/rc.d/renop` 或 `/usr/local/etc/rc.d/renop` 服务脚本                                 |

## 日常服务运维

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
