---
title: Cargo (Rust) 存储库
order: 2
category: 指南
description: 创建 Cargo 存储库、配置 Sparse Index、发布、所有权与 Cargodoc
---

# Cargo (Rust) 存储库指南

配置客户端前，应先创建格式为 `cargo` 的存储库。下方示例使用名称 `crates`。RenoP 实现 Cargo Sparse Index，
无需克隆 Git 索引即可流式提供 crate 归档。

## 配置 Cargo (`.cargo/config.toml`)

```toml
[registries.renop]
index = "sparse+http://localhost:3000/crates/"

# Optional: replace default crates.io upstream
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/crates/"
```

生产环境应使用 HTTPS。存储库 `config.json` 会声明下载与 API 路由。私有存储库设置 `auth-required`，索引与
crate 读取均要求凭据。

## 认证

应创建专用且可过期的 API Token。首次发布通常需要 `repository:read`、`repository:publish` 与
`package:create`；归档/yank 操作增加 `package:lifecycle`，管理所有者增加 `team:manage`。

```bash
cargo login --registry renop
# Paste your RenoP token when prompted
```

Cargo 将凭据保存到 `~/.cargo/credentials.toml`：

```toml
[registries.renop]
token = "your_renop_token"
```

Token 会作为完整 `Authorization` 值发送。RenoP 仍会将其权限与目标限制和账号当前存储库、包团队权限取交集。

## 依赖与发布

### 添加依赖 (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### 发布 crate

```bash
cargo publish --registry renop
```

首次成功发布会占用规范化名称，并授予发布者 L4。本地或适用的已启用上游镜像中已存在同名包时会被拒绝；
上游检查无法确定时安全返回 `503`，且不会占用名称。后续版本要求包团队具有发布权限。

### 搜索、yank 与 unyank

```bash
# Search crates
cargo search --registry renop my-crate

# Yank a version
cargo yank --registry renop --version 0.1.0 my-crate

# Unyank
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

包所有者可在包页面管理 L0-L4 协作者与邀请。镜像 crate 会标注其上游来源，不拥有本地所有者，并保持只读；
只有使用另一个确认可用的名称才能创建本地包。

## Cargodoc

上传文档后，RenoP 会校验并提取 rustdoc，在沙箱预览器中提供。需在 `config.yaml` 中启用 Cargodoc 并设置
大小限制。

访问地址：`http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
