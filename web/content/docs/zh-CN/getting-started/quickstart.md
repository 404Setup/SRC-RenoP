---
title: 快速开始
order: 3
category: 快速开始
description: 首次启动、管理员密码、默认仓库地址
---

# 快速开始

## 首次启动

首次启动时会自动创建 `admin` 账户。请在启动前通过环境变量设置管理员密码：

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='your-secure-password' ./renop
```

若未设置该环境变量，服务将自动生成随机密码并输出到日志中。启动完成后访问 `http://localhost:3000`。

使用 `admin` 账户登录后，具备 manager/admin 权限的用户可通过 Web 界面管理制品、用户、仓库和系统配置。

## 默认仓库

RenoP 默认提供三个仓库：

| 路径                              | 用途       |
|-----------------------------------|------------|
| `http://localhost:3000/releases`  | 正式版仓库 |
| `http://localhost:3000/snapshots` | 快照仓库   |
| `http://localhost:3000/private`   | 私有仓库   |

将上述地址配置到 Maven 的 `<repositories>` 或 `<distributionManagement>` 中即可使用。配置示例请参见 [Maven 客户端](./maven-client.md)。

## 健康检查

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## 环境变量

| 变量                           | 默认值              | 用途                                                           |
|--------------------------------|---------------------|----------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | 服务配置：监听地址、前端品牌、存储路径、更新器                 |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | 仓库配置：仓库列表、镜像配置、S3 存储                          |
| `RENOP_TOKENS`                 | `tokens.yaml`       | 账户与令牌配置                                                 |
| `RENOP_INDEX`                  | `index.json`        | 制品索引缓存                                                   |
| `RENOP_SESSIONS`               | `sessions.bin`      | 登录会话数据（protobuf 格式，旧版 `sessions.json` 自动迁移）   |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成            | 首个 admin 账户的初始密码                                      |

大部分配置项可在管理界面中修改。修改监听地址或 TLS 相关配置后，需要重启进程才能生效。

## 下一步

1. [配置](../configuration/overview.md) — 监听、TLS、品牌
2. [仓库与镜像](../configuration/repositories.md)
3. [Maven 客户端](./maven-client.md)
4. [HTTP API](../api/README.md)
