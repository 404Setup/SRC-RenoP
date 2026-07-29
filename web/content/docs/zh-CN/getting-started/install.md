---
title: 安装
order: 2
category: 快速开始
description: 下载 RenoP 二进制
---

# 安装

## 下载

网站 [下载](/download) 页：

- **正式版** — `https://mvnc.pkg.one/update/renop/stable/`（按平台 zip）
- **预览版** — `https://mvnc.pkg.one/update/renop/nightly/`

CI 写各通道的 `info.json`。更新说明从 GitHub 取。

平台与构建矩阵一致：Windows、Linux、FreeBSD、NetBSD、OpenBSD；amd64/arm64 及额外 Linux 架构。

## 从 zip 安装

1. 下载对应平台 zip
2. 解压到工作目录（配置写在进程 CWD 旁）
3. Windows 运行 `renop.exe`，其它系统运行 `./renop`

默认监听 `0.0.0.0:3000`。首次启动前设 `RENOP_DEFAULT_ADMIN_PASSWORD`，见 [快速开始](./quickstart.md)。

## 运行要求

- 可写工作目录（配置、会话、索引、默认本地存储）
- 用上游镜像或在线更新时需要出站 HTTPS
- 可选：前面加 nginx / Caddy 做 TLS

## 从源码构建

需要 **Go**、**PowerShell 7**、**Node.js**。

```powershell
pwsh ./build.ps1                 # 完整矩阵 → dist/
pwsh ./build.ps1 s               # Linux amd64/arm64、Windows amd64
pwsh ./build.ps1 c               # 当前平台
pwsh ./build.ps1 c nb            # 当前平台，不打 zip
```

会生成 protobuf、打包 `frontend/renop-html`、嵌入资源、`CGO_ENABLED=0` 编译。细节见仓库 `README.md`。
