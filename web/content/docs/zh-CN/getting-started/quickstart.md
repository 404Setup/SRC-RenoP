---
title: 快速开始
order: 3
category: 快速开始
description: 首次启动、管理员密码与默认仓库地址
---

# 快速开始

## 首次启动

首次启动会创建 `admin` 账户。请在启动前设置密码：

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

若未设置该变量，随机密码会打印到服务器日志。启动后打开 `http://localhost:3000`。

使用用户名 `admin` 与上述密码登录。管理员可在 Web UI 中浏览制品、管理用户、仓库与设置。

## 默认仓库

| 路径                              | 用途     |
|-----------------------------------|----------|
| `http://localhost:3000/releases`  | 正式制品 |
| `http://localhost:3000/snapshots` | 快照制品 |
| `http://localhost:3000/private`   | 私有制品 |

将上述 URL 写入 Maven 的 `<repositories>` 或 `<distributionManagement>`
。完整示例见 [Maven 客户端配置](./maven-client.md)。

## 健康检查

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

## 环境变量

| 变量                           | 默认                | 用途                           |
|--------------------------------|---------------------|--------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | 服务器、前端、存储路径、更新器 |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | 仓库、镜像、按仓库 S3          |
| `RENOP_TOKENS`                 | `tokens.yaml`       | 账户与访问 Token               |
| `RENOP_INDEX`                  | `index.json`        | 制品索引                       |
| `RENOP_SESSIONS`               | `sessions.json`     | 登录会话                       |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成            | 首个管理员密码                 |

多数设置也可在管理 UI 中修改。修改监听地址或 TLS 后需重启服务。

## 下一步

1. [配置](../configuration/overview.md) 监听、TLS 与品牌
2. 定义 [仓库与镜像](../configuration/repositories.md)
3. 配置 [Maven 客户端](./maven-client.md)
4. 查阅 [HTTP API](../api/README.md)
