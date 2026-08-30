---
title: 项目介绍
order: 1
category: 快速开始
description: RenoP 是一套集成式自托管包发布平台
---

# RenoP 项目介绍

RenoP 是一套集成式、自托管的包发布与分发服务。其产品模型更接近私有化 Central，而不是仅提供 Maven 文件
树的软件：单个 Go 进程内置管理界面、身份、团队、验证工作流、包目录、镜像、存储、审计与更新。

## 支持的协议

- **Maven / Gradle**：全局已验证发布域、现代域目录、经典布局兼容、Maven 2 客户端路径、镜像、Javadoc 与
  OpenPGP 分离签名校验。
- **Cargo**：Sparse Index、显式包所有权、发布、搜索、yank/unyank、镜像与 Cargodoc。
- **npm**：显式预留软件包、不可变版本、作用域私有包、发布标签、团队与镜像。
- **Docker / OCI**：Distribution v2、镜像预创建、私有镜像团队、分块 Blob、跨仓库挂载、多架构 Manifest 与镜像。
- **Files**：支持覆盖与镜像的非结构化文件存储，不生成 Maven 元数据，也不执行签名工作流。

## 存储与数据库

- **存储**：流式本地 Disk，或存储库独立的 S3 兼容对象存储。
- **数据库**：默认使用内嵌 SQLite，也支持外部 MySQL 与 PostgreSQL。
- **一致性**：存储库门控协调上传、删除、镜像提交、GPG 发布及引擎/存储变更，不会将大型对象完整读入内存。

## 核心能力

| 能力           | 说明                                                        |
|:---------------|:------------------------------------------------------------|
| **单一服务**   | 内嵌前端与协议 API，无需独立应用运行时                      |
| **全局身份**   | 使用用户名公开个人资料，内部使用不可变用户 ID               |
| **细粒度权限** | 存储库权限、L0-L4 包/域团队、可限定目标和有效期的 API Token |
| **验证发布**   | Maven 域所有权、上游名称冲突检查与可选 OpenPGP 隔离队列     |
| **运维能力**   | 原生系统服务、计划任务、持久审计/消息与原地更新             |
| **安全防护**   | 有界流式处理、速率限制、异常封禁、可信代理与沙箱文档预览    |

## 文档导航

- [安装](./install.md) — 发布包、平台选择与源码构建
- [快速开始](./quickstart.md) — 首次启动、管理员与存储库创建
- [系统架构](./architecture.md) — 模块、授权、存储与异步任务
- [配置概览](../configuration/overview.md) — 已校验设置与环境变量
- [Maven 与 Gradle](../guides/maven-client.md) — 已验证域与 JVM 客户端
- [Cargo](../guides/cargo-registry.md) — Sparse Registry 与 crate 生命周期
- [Docker 与 OCI](../guides/docker-registry.md) — 镜像预创建、登录、推送与拉取
- [npm 存储库](../guides/npm-registry.md) — 软件包预留、客户端配置、发布与团队
