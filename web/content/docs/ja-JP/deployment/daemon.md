---
title: システムサービス管理
order: 1
category: デプロイと運用
description: --install および --uninstall によるシステムサービス登録
---

# システムサービス管理

```bash
# システムサービスとして登録
./renop --install

# サービスを削除
./renop --uninstall
```

Windows サービス (SCM)、Linux systemd/OpenRC、macOS launchd、BSD rc.d に自動対応します。
