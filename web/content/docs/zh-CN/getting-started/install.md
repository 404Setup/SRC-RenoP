---
title: 安装
order: 2
category: 快速开始
description: 下载 RenoP 二进制
---

# 安装

## 下载

访问 [下载页](/download) 或直接从以下地址获取 zip 包：

- 稳定版：`https://mvnc.pkg.one/update/renop/stable/`
- 开发版：`https://mvnc.pkg.one/update/renop/nightly/`

## 从 zip 包安装

1. 将 zip 包解压到目标工作目录
2. Windows 系统执行 `renop.exe`，其他系统执行 `./renop`

服务默认监听 `0.0.0.0:3000`。首次启动前建议设置环境变量 `RENOP_DEFAULT_ADMIN_PASSWORD`，详见[快速开始](./quickstart.md)。

## 从源码构建

从源码构建时，必须使用 [我们维护的 Go fork](https://github.com/404Setup/go/releases)，不可使用官方 Go 发行版。此外还需安装 PowerShell 7 和 Node.js。

1. 查看 `go.mod` 中指定的 `go` 版本
2. 在 releases 页面找到对应的 `go<版本>` tag，下载适合当前系统架构的文件
3. 使用同一 release 中提供的 `SHA256SUMS` 文件校验下载文件的完整性
4. 解压后将环境变量 `GOROOT` 指向 `go` 目录，将 `GOROOT/bin` 添加到 `PATH`，然后运行 `go version` 确认版本正确

```powershell
pwsh ./build.ps1      # 完整交叉编译（所有平台）
pwsh ./build.ps1 s    # 主流平台（Linux/Windows amd64/amd64v4/arm64）
pwsh ./build.ps1 c    # 仅当前平台
pwsh ./build.ps1 c nb # 当前平台，无打包（直接输出二进制）
```

详细构建说明请参阅仓库根目录的 `README.md`。
