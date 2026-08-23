---
title: Cargo (Rust) 注册源
order: 2
category: 客户端指南
description: 配置 Rust Cargo 稀疏索引、发布 Crate 与使用 Cargodoc 在线文档
---

# Cargo (Rust) 注册源配置

RenoP 原生支持现代 Cargo 稀疏索引（Sparse Index）协议（Cargo 1.68+ 默认启用），无需克隆整个 Git 索引仓库即可快速拉取与发布
Crate。

## 1. 配置 Cargo 客户端 (`.cargo/config.toml`)

在项目根目录或全局用户目录（`~/.cargo/config.toml`）中添加以下配置：

```toml
[registries.renop]
index = "sparse+http://localhost:3000/releases/index/"

# 如果希望将 RenoP 设为默认 crates.io 替代源（可选）
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/releases/index/"
```

> **说明**：URL 格式为 `sparse+<http或https>://<host>:<port>/<repo>/index/`。

## 2. 配置认证 Token

在发布 Crate 或拉取私有 Crate 时，需要在 Cargo 中登录并设置 Token：

```bash
cargo login --registry renop
# 终端提示输入 token 时，粘贴在 RenoP Web 界面「令牌管理」中创建的 PAT 或上传令牌
```

或者直接在 `~/.cargo/credentials.toml` 中配置：

```toml
[registries.renop]
token = "your_renop_token"
```

## 3. 使用与发布 Crate

### 在项目中引入依赖 (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### 发布 Crate

在 Crate 源码目录下执行发布指令：

```bash
cargo publish --registry renop
```

### 检索与撤回 Crate

```bash
# 检索 Crate
cargo search --registry renop my-crate

# 撤回指定版本
cargo yank --registry renop --version 0.1.0 my-crate

# 取消撤回
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

## 4. Cargodoc 在线文档预览

当上传附带文档或通过 RenoP 构建的 Crate 时，可以通过内置的 Cargodoc 引擎在线查看 rustdoc 生成的 HTML 文档：

访问格式：
`http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
