---
title: 配置概览
order: 1
category: 配置
description: 配置文件、服务端设置与环境变量
---

# 配置概览

RenoP 将配置与状态保存在进程工作目录，路径可通过环境变量覆盖。

## 文件

| 文件                | 环境变量             | 用途                                  |
|---------------------|----------------------|---------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | 监听、TLS、前端品牌、存储路径、更新器 |
| `repositories.yaml` | `RENOP_REPOSITORIES` | 仓库、镜像、按仓库 S3                 |
| `tokens.yaml`       | `RENOP_TOKENS`       | 用户、角色、上传 Token                |
| `index.json`        | `RENOP_INDEX`        | 制品索引缓存                          |
| `sessions.json`     | `RENOP_SESSIONS`     | 浏览器登录会话                        |

运行时相关：

| 变量                           | 默认     | 用途                  |
|--------------------------------|----------|-----------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成 | 首个 `admin` 账户密码 |

## `config.yaml` 结构

### `storage_path`

本地制品存储根目录，常见默认值为 `storage`。

### `server`

| 键                    | 默认              | 说明                                              |
|-----------------------|-------------------|---------------------------------------------------|
| `host`                | `0.0.0.0`         | 监听地址                                          |
| `port`                | `3000`            | 监听端口                                          |
| `ssl_enabled`         | `false`           | 是否启用 TLS                                      |
| `ssl_cert_path`       | `""`              | 证书路径                                          |
| `ssl_key_path`        | `""`              | 私钥路径                                          |
| `domain`              | `localhost`       | 对外域名（部分 UI/元数据）                        |
| `enable_compression`  | `false`           | HTTP 压缩                                         |
| `file_cache_size_mb`  | `100`             | 内存文件缓存（MB）                                |
| `max_active_requests` | `2000`            | 并发请求上限（超限返回 503）                      |
| `trusted_proxies`     | `[]`              | 额外可信反向代理 CIDR/IP（环回始终可信）          |
| `cdn_ip_header`       | `X-Forwarded-For` | 可信代理后取真实 IP 的头（如 `CF-Connecting-IP`） |

修改 host、port 或 TLS 后需 **重启**进程。

### `frontend`

嵌入式仓库浏览器的品牌信息：`id`、`title`、`description`、`organization_website`、`organization_logo`、`background_url`、
`icp_license` 等。

### `updater`

| 键        | 默认      | 说明                           |
|-----------|-----------|--------------------------------|
| `channel` | `release` | `release` 或 `nightly`         |
| `mode`    | `manual`  | 更新应用方式（如在 UI 中手动） |

官网[下载](/download)页与实例更新使用同一类发行源。

## 管理界面

**manager** / **admin** 可在 **设置**、 **仓库** 中修改多数配置。

## 存储后端

- **本地磁盘**（`storage_path`，默认）
- **兼容 S3 的对象存储**（在 `repositories.yaml` 中按仓库配置）

上传时可生成 MD5 / SHA-1 / SHA-256 / SHA-512 校验和旁路文件。

详见 [仓库与镜像](./repositories.md)。

## 相关

- [快速开始](../getting-started/quickstart.md)
- [Maven 客户端](../getting-started/maven-client.md)
- [API 索引](../api/README.md)
