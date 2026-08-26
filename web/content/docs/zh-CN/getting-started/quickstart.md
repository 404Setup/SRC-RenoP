---
title: 快速开始
order: 3
category: 快速开始
description: 首次启动、管理员初始化、健康检查与存储库创建
---

# 快速开始

## 启动服务

首次启动时，RenoP 会在数据库中创建 `admin` 系统超级管理员。建议显式设置密码：

```bash
# Linux / macOS
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop

# Windows (PowerShell)
$env:RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password'
.\renop.exe
```

未设置时，RenoP 会生成随机密码并只在 stdout 输出一次。应立即保存，然后打开 `http://localhost:3000`。
服务默认监听 `0.0.0.0:3000`；生产环境应使用 TLS 或可信反向代理。

## 默认与新建存储库

初始 `repositories.yaml` 包含三个用于向后兼容的 Maven 存储库：

| 路径 | 可见性 | 策略 |
|:-----|:-------|:-----|
| `/releases` | `PUBLIC` | Maven，禁止重复发布 |
| `/snapshots` | `PUBLIC` | Maven，允许重复发布 |
| `/private` | `PRIVATE` | Maven，要求认证 |

Cargo、Docker 与 `files` 存储库需要在仓库管理中显式创建。Docker 镜像与 Cargo 名称同样是显式资源，仅在
上游名称检查成功后才能创建或首次发布。Maven 发布还要求账号菜单中已有验证通过的域。

## 健康检查

```bash
curl -s http://localhost:3000/api/status/health
# Output: "UP"
```

protobuf 运行时指标位于 `/api/status/instance`。健康检查只说明进程正在响应；接收生产流量前，应通过一次真实的
认证操作验证数据库与存储。

## 重要环境变量

| 变量 | 默认值 | 用途 |
|:-----|:-------|:-----|
| `RENOP_CONFIG` | `config.yaml` | 主配置文件路径 |
| `RENOP_REPOSITORIES` | `repositories.yaml` | 存储库配置文件路径 |
| `RENOP_INDEX` | `index.json` | 持久化文件索引快照路径 |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 首次生成 | `admin` 不存在时的初始密码 |

账号、会话、团队、API Token、行为日志与消息属于数据库数据，没有对应 YAML 路径变量。

## 后续步骤

- [配置概览](../configuration/overview.md) — TLS、数据库、代理、文档预览与更新器
- [存储库与镜像](../configuration/repositories.md) — 引擎、可见性、上游、迁移与 S3
- [Maven 与 Gradle](../guides/maven-client.md) — 验证发布域并配置 JVM 客户端
- [Cargo 存储库](../guides/cargo-registry.md) — 创建 Cargo 存储库并发布 crate
- [Docker 存储库](../guides/docker-registry.md) — 推送前创建镜像并配置 Docker 或 Podman
