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

源码构建必须使用[我们维护的 Go fork](https://github.com/404Setup/go/releases)，不能使用官方 Go。还需要 PowerShell 7 和
Node.js。

1. 查看 `go.mod` 中的 `go` 版本。
2. 在 releases 中找到最新的 `go<版本>` tag，下载当前系统和架构对应的文件。
3. 使用同一 release 中的 `SHA256SUMS` 校验文件。
4. 解压后将 `GOROOT` 指向 `go` 目录，把 `GOROOT/bin` 加入 `PATH`，再运行 `go version` 确认版本。

```powershell
pwsh ./build.ps1
pwsh ./build.ps1 s
pwsh ./build.ps1 c
pwsh ./build.ps1 c nb
```

详见仓库 `README.md`。
