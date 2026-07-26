---
title: 简介
order: 1
category: 快速开始
description: RenoP 是什么，适合谁
---

# 简介

RenoP 是一款轻量、可快速部署的 **自托管 Maven 服务器**，面向个人与团队。

它专注于：

- 开箱即用的默认配置
- Release、Snapshot 与 Private 仓库
- 带本地缓存的 Maven 镜像代理
- 小而清晰的 Web 界面：浏览、上传、用户、Token 与健康状态

若你计划将其用于 **公开托管**，目前这并不是 RenoP 的主要目标场景。

## 设计目标

| 目标       | 含义                               |
|------------|------------------------------------|
| 运维简单   | 单一二进制，配置与状态放在工作目录 |
| Maven 原生 | 标准仓库布局与客户端兼容           |
| 透明       | 无广告、无产品遥测，社区版免费     |

## 功能要点

- **Release / Snapshot / Private** 仓库与 Maven 布局
- **上游镜像**（本地缓存与负缓存）
- **Web UI**：浏览、上传、用户、Token、健康状态
- **本地磁盘或 S3 兼容**对象存储
- **认证**：会话、Basic、Bearer / 上传 Token、仓库权限
- **附加能力**：校验和、Javadoc 浏览、在线更新、分块上传 API

## 下一步

1. [安装](./install.md) 正式版或预览版
2. 阅读 [快速开始](./quickstart.md)
3. 配置 [Maven 客户端](./maven-client.md)
4. 需要更多控制时查看 [配置说明](../configuration/overview.md)
