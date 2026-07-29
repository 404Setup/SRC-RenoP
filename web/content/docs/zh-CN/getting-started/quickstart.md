---
title: 快速开始
order: 3
category: 快速开始
description: 首次启动、管理员密码、默认仓库地址
---

# 快速开始

## 首次启动

首次启动时会创建 `admin` 账户。请在启动前通过环境变量设置密码：

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

若未设置该变量，服务会生成随机密码并写入服务器日志。启动完成后访问 `http://localhost:3000`。

使用 `admin` 登录。具备 manager / admin 权限的账户可在 Web 界面中管理制品、用户、仓库与系统设置。

## 默认仓库

| 路径                              | 用途   |
|-----------------------------------|--------|
| `http://localhost:3000/releases`  | 正式版 |
| `http://localhost:3000/snapshots` | 快照   |
| `http://localhost:3000/private`   | 私有   |

将上述地址配置到 Maven 的 `<repositories>` 或 `<distributionManagement>`。示例见 [Maven 客户端](./maven-client.md)。

## 健康检查

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## 环境变量

| 变量                           | 默认                | 用途                                                  |
|--------------------------------|---------------------|-------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | 服务、前端、存储、更新器                              |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | 仓库、镜像、按仓库 S3                                 |
| `RENOP_TOKENS`                 | `tokens.yaml`       | 账户与 Token                                          |
| `RENOP_INDEX`                  | `index.json`        | 制品索引                                              |
| `RENOP_SESSIONS`               | `sessions.bin`      | 登录会话（protobuf；旧版 `sessions.json` 会自动迁移） |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成            | 首个 admin 密码                                       |

多数配置项也可在管理界面中修改。修改监听地址或 TLS 相关配置后，需要重启进程。

## 下一步

1. [配置](../configuration/overview.md) — 监听、TLS、品牌
2. [仓库与镜像](../configuration/repositories.md)
3. [Maven 客户端](./maven-client.md)
4. [HTTP API](../api/README.md)
