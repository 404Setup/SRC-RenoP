---
title: 快速开始
order: 3
category: 快速开始
description: 首次启动、管理员密码、默认仓库地址
---

# 快速开始

## 首次启动

首次启动会创建 `admin`。启动前设密码：

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

不设的话，随机密码打在服务器日志里。然后打开 `http://localhost:3000`。

用 `admin` 登录。管理员可在 Web UI 管制品、用户、仓库和设置。

## 默认仓库

| 路径                              | 用途   |
|-----------------------------------|--------|
| `http://localhost:3000/releases`  | 正式版 |
| `http://localhost:3000/snapshots` | 快照   |
| `http://localhost:3000/private`   | 私有   |

写进 Maven 的 `<repositories>` 或 `<distributionManagement>`。示例：[Maven 客户端](./maven-client.md)。

## 健康检查

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## 环境变量

| 变量                           | 默认                | 用途                     |
|--------------------------------|---------------------|--------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | 服务、前端、存储、更新器 |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | 仓库、镜像、按仓库 S3    |
| `RENOP_TOKENS`                 | `tokens.yaml`       | 账户与 Token             |
| `RENOP_INDEX`                  | `index.json`        | 制品索引                 |
| `RENOP_SESSIONS`               | `sessions.json`     | 登录会话                 |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成            | 首个 admin 密码          |

多数也能在管理 UI 里改。改监听地址或 TLS 后要重启。

## 下一步

1. [配置](../configuration/overview.md) — 监听、TLS、品牌
2. [仓库与镜像](../configuration/repositories.md)
3. [Maven 客户端](./maven-client.md)
4. [HTTP API](../api/README.md)
