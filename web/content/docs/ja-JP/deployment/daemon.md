---
title: システムサービス管理
order: 1
category: デプロイ
description: --install と --uninstall による RenoP のネイティブサービス登録
---

# システムサービス管理

RenoP は外部 wrapper を使わず、自動起動する OS ネイティブサービスとして登録できます。

## 1. コマンド

```bash
# Register and start as a system service
./renop --install

# Stop and remove the system service
./renop --uninstall

# View CLI help
./renop --help
```

`--install` はバイナリの絶対パスを記録し、そのディレクトリを作業ディレクトリにします。`/opt/renop` や
`C:\Program Files\RenoP` など、最終配置先から実行してください。

## 2. 対応プラットフォーム

| OS | Service manager | 動作 |
|:---|:----------------|:-----|
| **Windows** | SCM | 自動起動の `RenoP` service を登録し、`services.msc` で管理 |
| **Linux (systemd)** | systemd | `/etc/systemd/system/renop.service` を作成して有効化 |
| **Linux (OpenRC)** | OpenRC | `/etc/init.d/renop` を作成して default runlevel に追加 |
| **macOS** | launchd | `/Library/LaunchDaemons/one.pkg.renop.plist` を作成して load |
| **BSD** | rc.d | OS に適した rc.d script を生成 |

install/uninstall には管理権限が必要です。登録済みパスを維持するため、バイナリの移動や置換は正規の更新手順で
実施してください。

## 3. 日常操作

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
