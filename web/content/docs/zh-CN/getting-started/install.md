---
title: 安装与构建
order: 2
category: 快速开始
description: 下载二进制、微架构选型与源码编译
---

# 安装与构建

## 1. 下载预编译二进制

你可以从 Web 界面中的 [下载页面](/download) 或以下地址获取纯 Brotli 预编译更新包：

- **稳定版 (Stable)**：`https://mvnc.pkg.one/update/renop/stable/`
- **开发版 (Nightly)**：`https://mvnc.pkg.one/update/renop/nightly/`

## 2. x86-64 微架构等级说明

RenoP 针对 64 位 x86 架构提供了不同指令集等级的构建版本：

| 等级                  | 指令集支持                          | 适用场景                                                                         |
|:----------------------|:------------------------------------|:---------------------------------------------------------------------------------|
| **x86-64-v1**         | 通用 64 位基线指令集                | 兼容所有 64 位 x86 处理器，适用于较旧的 CPU 或老旧虚拟机                         |
| **x86-64-v2**         | SSE3, SSSE3, SSE4.1, SSE4.2, POPCNT | 适用于 2008 年及之后的主流 Intel 与 AMD 处理器                                   |
| **x86-64-v3**（推荐） | AVX, AVX2, BMI1, BMI2, FMA3         | 适用于 Intel Haswell (2013+)、AMD Zen 2 (2019+) 及更新的处理器，推荐生产环境使用 |
| **x86-64-v4**         | AVX-512 基础与扩展指令集            | 适用于支持 AVX-512 的服务器级 CPU（如 Intel Skylake-X/Ice Lake, AMD Zen 4 等）   |
| **ARM64**             | NEON, Crypto                        | 适用于 Apple Silicon、AWS Graviton 及各类 ARM64 Linux 服务器                     |

## 3. 校验与运行

通道 `info.json` 会为每个平台包提供 SHA-256。建议在解压 `.br` 前校验文件完整性：

```bash
# Linux
sha256sum -c SHA256SUMS --ignore-missing

# Windows (PowerShell)
Get-FileHash -Algorithm SHA256 .\renop-windows-amd64v3.br
```

将纯 Brotli 流解压为 `renop` 或 `renop.exe` 后直接运行。下载页面也可以完全在浏览器中将新版 `.br`
转换为旧版 ZIP 布局。

- **Linux / macOS**：`./renop`
- **Windows**：`.\renop.exe`

服务默认监听 `0.0.0.0:3000`。首次启动建议先设置管理员密码，参见 [快速开始](./quickstart.md)。

## 4. 注册为系统服务

RenoP 内置了跨平台的系统服务安装与卸载功能：

```bash
# 安装并注册为开机自启系统服务
./renop --install

# 停止并移除系统服务
./renop --uninstall
```

支持 Windows 服务（SCM）、Linux systemd / OpenRC、macOS LaunchDaemons 及 BSD
rc.d。详细说明请参考 [系统服务管理](../deployment/daemon.md)。

## 5. 从源码构建

如需从源码编译 RenoP，需要准备以下环境：

- **Go 编译器**：请使用我们维护的 [404Setup/go](https://github.com/404Setup/go/releases) 分支（不可使用 Go 官方标准发行版）。
- **前端工具**：Node.js 18+ 与 pnpm。
- **脚本环境**：PowerShell 7 (`pwsh`)。
- **Protobuf**：`protoc` 与 `protoc-gen-go`（用于更新 API 协议定义）。

### 构建步骤

```powershell
# 1. 确保 GOROOT 指向 404Setup/go 目录
$env:GOROOT = "D:\tools\go"
$env:PATH = "$env:GOROOT\bin;$env:PATH"

# 2. 安装依赖并构建前端
pnpm install --frozen-lockfile
pnpm run build:frontend

# 3. 编译二进制
pwsh ./build.ps1 c nb    # 仅编译当前平台，直接输出二进制（不压缩打包）
pwsh ./build.ps1 c       # 仅编译当前平台并生成纯 Brotli 更新包
pwsh ./build.ps1 s       # 编译主流平台 (Linux/Windows amd64/amd64v3/arm64)
pwsh ./build.ps1         # 完整全平台矩阵交叉编译
```
