---
title: 简介
order: 1
category: 快速开始
description: RenoP 概述、支持协议与功能特性
---

# RenoP 简介

RenoP 是一个自托管的多协议软件包与制品仓库服务器。它使用 Go 语言开发，内嵌单页 Web 管理界面，旨在提供轻量、低依赖、易于部署的私有化制品托管服务。

## 支持的协议与生态

- **Maven / Gradle**：支持 Release、Snapshot、Private 仓库，遵循标准 Maven 目录布局，支持 Javadoc 在线预览与 GPG 签名校验。
- **Cargo (Rust)**：支持 Cargo 稀疏索引（Sparse Index）协议、Crate 发布、下载、检索与撤回，支持 crates.io 镜像代理以及 Cargodoc
  文档在线查看。
- **Docker / OCI 镜像**：实现 OCI Distribution Spec v2 与 Docker Registry v2 规范，支持多架构镜像清单、分块 Blob 上传与上游镜像代理。

## 存储与数据库支持

- **存储后端**：支持本地文件系统存储，或连接 AWS S3、MinIO、Cloudflare R2、阿里云 OSS 等 S3 兼容对象存储。
- **数据库**：内置 SQLite；同时支持外部 MySQL 8.0+ 与 PostgreSQL 数据库。

## 核心特性

| 特性               | 说明                                                                                              |
|:-------------------|:--------------------------------------------------------------------------------------------------|
| **单二进制部署**   | 无需额外安装运行环境，内置 Web 界面，开箱即用                                                     |
| **上游镜像代理**   | 代理上游 Maven、Cargo 与 Docker 仓库，支持本地缓存、负缓存与按规则过滤                            |
| **细粒度权限控制** | 支持基于角色的访问控制（RBAC）、仓库级权限（读/写/管理）与个人访问令牌（PAT）                     |
| **系统服务守护**   | 内置 `--install` 与 `--uninstall` 命令，支持 Windows 服务、systemd、OpenRC、LaunchDaemons 与 rc.d |
| **安全与防御**     | 支持 Detached GPG 签名验证、API 请求限流与异常 IP 拦截                                            |

## 快速导航

- [安装指南](./install.md) — 预编译包下载、微架构选择与编译方法
- [快速开始](./quickstart.md) — 启动服务、初始化管理员密码与默认仓库
- [系统架构](./architecture.md) — 内部模块与核心设计说明
- [配置说明](../configuration/overview.md) — 配置文件与环境变量
- [Maven 客户端](../guides/maven-client.md) — Maven 与 Gradle 接入配置
- [Cargo 注册源](../guides/cargo-registry.md) — Rust / Cargo 接入配置
- [Docker 镜像库](../guides/docker-registry.md) — Docker 与 Podman 接入配置
