---
title: 安装
order: 2
category: 快速开始
description: 下载 RenoP 二进制
---

# 安装

## 下载

[下载页](/download)，或直接取 zip：

- 正式版：`https://mvnc.pkg.one/update/renop/stable/`
- 预览版：`https://mvnc.pkg.one/update/renop/nightly/`

## 从 zip 安装

1. 解压到工作目录
2. Windows 运行 `renop.exe`，其它系统 `./renop`

默认 `0.0.0.0:3000`。首次启动前设 `RENOP_DEFAULT_ADMIN_PASSWORD`，见 [快速开始](./quickstart.md)。

## 从源码构建

需要 Go、PowerShell 7、Node.js。

```powershell
pwsh ./build.ps1                 # 完整矩阵 → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64、Windows amd64
pwsh ./build.ps1 c               # 当前平台
pwsh ./build.ps1 c nb            # 当前平台，不打 zip
```

详见仓库 `README.md`。
