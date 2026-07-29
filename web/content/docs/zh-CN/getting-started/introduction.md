---
title: 简介
order: 1
category: 快速开始
description: RenoP 是什么
---

# 简介

RenoP 是一个自托管的 Maven 服务器。

- Release / Snapshot / Private 仓库
- 带本地缓存的上游镜像代理
- Web 界面：浏览、上传、用户、Token、健康检查

暂时不做公开多租户托管。

## 目标

| 目标       | 说明                             |
|------------|----------------------------------|
| 运维简单   | 单个二进制，配置放在工作目录     |
| Maven 布局 | 标准仓库路径，普通客户端可直接用 |
| 无多余东西 | 无广告、无产品遥测、免费         |

## 功能

- **Release / Snapshot / Private** 仓库（Maven 布局）
- **上游镜像**（本地缓存、负缓存）
- **Web UI**：浏览、上传、用户、Token、健康状态
- **本地磁盘或 S3 兼容**存储
- **认证**：会话、Basic、Bearer / 上传 Token、仓库权限
- **其它**：校验和、Javadoc、在线更新、分块上传 API

## 下一步

1. [安装](./install.md)
2. [快速开始](./quickstart.md)
3. [Maven 客户端](./maven-client.md)
4. [配置说明](../configuration/overview.md)
