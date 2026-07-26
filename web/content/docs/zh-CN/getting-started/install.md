---
title: 安装
order: 2
category: 快速开始
description: 下载并放置 RenoP 可执行文件
---

# 安装

## 官方下载

请使用网站 [下载](/download) 页面：

- **正式版** — 官方源 `https://mvnc.pkg.one/update/renop/stable/`（按平台分 zip）
- **预览版** — 官方源 `https://mvnc.pkg.one/update/renop/nightly/`（按平台分 zip）

元数据由 CI 写入各通道的 `info.json`。更新说明仍从 GitHub 获取。

支持的平台与项目构建矩阵一致（Windows、Linux、FreeBSD、NetBSD、OpenBSD；amd64/arm64 及额外 Linux 架构）。

## 从发行包安装

1. 下载对应平台的 zip
2. 解压到工作目录（配置文件会创建在进程工作目录旁）
3. Windows 运行 `renop.exe`，类 Unix 系统运行 `./renop`

默认监听 `0.0.0.0:3000`。首次启动前请设置 `RENOP_DEFAULT_ADMIN_PASSWORD`（见 [快速开始](./quickstart.md)）。

## 运行环境

- 可写的工作目录（配置、会话、索引，以及默认的本地存储）
- 若使用上游镜像或在线更新，需可访问外网 HTTPS
- 可选：在 RenoP 前用 nginx / Caddy 等反向代理终结 TLS

## 从源码构建

需要 **Go**、 **PowerShell 7** 与 **Node.js**（前端 Rolldown 打包）。

```powershell
pwsh ./build.ps1                 # 完整目标矩阵，打包到 dist/
pwsh ./build.ps1 s               # Linux amd64/arm64 与 Windows amd64
pwsh ./build.ps1 c               # 仅当前平台
pwsh ./build.ps1 c nb            # 当前平台，不打 zip
```

构建会生成 Protocol Buffer 代码、打包 `frontend/renop-html`、嵌入资源并以 `CGO_ENABLED=0` 编译。protobuf 与仅前端重建步骤见仓库
`README.md`。
