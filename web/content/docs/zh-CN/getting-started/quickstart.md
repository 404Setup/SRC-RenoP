---
title: 快速开始
order: 3
category: 快速开始
description: 首次运行、设置管理员密码与默认仓库地址
---

# 快速开始

## 1. 首次启动

首次启动时，RenoP 会自动创建 `admin` 账户。建议在启动前通过环境变量指定管理员密码：

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

如果未设置该环境变量，程序将自动生成一条随机密码并在控制台日志中输出。

启动后访问 `http://localhost:3000` 进入 Web 管理界面。

## 2. 默认仓库地址

RenoP 启动后默认初始化以下仓库：

| 仓库路径                          | 可见性    | 用途                                       |
|:----------------------------------|:----------|:-------------------------------------------|
| `http://localhost:3000/releases`  | `PUBLIC`  | Maven 正式版仓库（默认禁止覆盖同版本制品） |
| `http://localhost:3000/snapshots` | `PUBLIC`  | Maven 快照仓库（允许覆盖部署）             |
| `http://localhost:3000/private`   | `PRIVATE` | Maven 私有仓库（需认证才能访问）           |

此外，Cargo 稀疏索引和 Docker 镜像端点分别为：

- Cargo 索引端点：`http://localhost:3000/index/`（或各仓库对应路径）
- Docker 端点：`http://localhost:3000/v2/`

## 3. 健康检查

你可以通过 HTTP 接口探测服务是否正常运行：

```bash
curl -s http://localhost:3000/api/status/health
# 输出: "UP"
```

## 4. 环境变量列表

| 变量名                         | 默认值              | 说明                                             |
|:-------------------------------|:--------------------|:-------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | 主配置文件路径                                   |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | 仓库与镜像代理配置文件路径                       |
| `RENOP_TOKENS`                 | `tokens.yaml`       | 初始用户与静态令牌文件（启动后自动同步到数据库） |
| `RENOP_INDEX`                  | `index.json`        | 制品搜索索引缓存文件                             |
| `RENOP_SESSIONS`               | `sessions.bin`      | 会话数据文件（protobuf 格式）                    |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成            | 初始管理员密码                                   |

## 5. 后续配置

- [配置概览](../configuration/overview.md) — 端口、TLS、数据库与存储配置
- [仓库与镜像配置](../configuration/repositories.md) — 新增仓库、设置上游代理与 S3
- [Maven 客户端指南](../guides/maven-client.md) — 配置 Maven 与 Gradle
- [Cargo 注册源指南](../guides/cargo-registry.md) — 配置 Rust `config.toml`
- [Docker 镜像库指南](../guides/docker-registry.md) — 配置 Docker / Podman
