---
title: 配置概览
order: 1
category: 配置
description: 配置文件、服务端设置与环境变量
---

# 配置概览

配置文件与运行时状态保存在进程工作目录中。相关路径可通过环境变量覆盖。

## 文件

| 文件                | 环境变量             | 用途                                                 |
|---------------------|----------------------|------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | 监听、TLS、前端品牌、存储路径、数据库、更新器        |
| `repositories.yaml` | `RENOP_REPOSITORIES` | 仓库、镜像、按仓库 S3                                |
| `tokens.yaml`       | `RENOP_TOKENS`       | 用户、角色、上传 Token（启动时自动迁移至数据库）     |
| `renop.db`          | —                    | 内嵌 SQLite 数据库（存储用户 Token 与 Session 会话） |
| `index.json`        | `RENOP_INDEX`        | 制品索引缓存                                         |
| `sessions.bin`      | `RENOP_SESSIONS`     | 浏览器登录会话（加载时自动迁移至数据库）             |

运行时相关：

| 变量                           | 默认     | 用途                  |
|--------------------------------|----------|-----------------------|
| `RENOP_DEFAULT_ADMIN_PASSWORD` | 自动生成 | 首个 `admin` 账户密码 |

## `config.yaml` 结构

### 全局存储与 Javadoc 配置

| 键                       | 默认      | 说明                                     |
|--------------------------|-----------|------------------------------------------|
| `storage_path`           | `storage` | 本地制品存储的根目录                     |
| `enable_javadoc_preview` | `true`    | 是否启用 Javadoc 在线预览功能            |
| `javadoc_extract_path`   | `""`      | Javadoc 解压提取目录（留空使用默认缓存） |
| `max_javadoc_size_mb`    | `48`      | Javadoc 解压文件最大体积限制（MB）       |

### `server`

| 键                    | 默认              | 说明                                                        |
|-----------------------|-------------------|-------------------------------------------------------------|
| `host`                | `0.0.0.0`         | 监听地址                                                    |
| `port`                | `3000`            | 监听端口                                                    |
| `ssl_enabled`         | `false`           | 是否启用 TLS                                                |
| `ssl_cert_path`       | `""`              | TLS 证书路径                                                |
| `ssl_key_path`        | `""`              | TLS 私钥路径                                                |
| `domains`             | `[localhost]`     | 本实例对外主机名（用于 UI/元数据，以及默认 CORS）           |
| `cors_origins`        | `[]`              | 浏览器 CORS 允许列表（空 = 仅 `domains`；`*` = 全部）       |
| `enable_compression`  | `false`           | 是否启用 HTTP 响应压缩                                      |
| `file_cache_size_mb`  | `16`              | 内存文件缓存大小（MB）                                      |
| `max_active_requests` | `512`             | 并发请求上限（超限返回 503）                                |
| `trusted_proxies`     | `[]`              | 额外可信反向代理的 CIDR/IP（环回地址始终可信）              |
| `cdn_ip_header`       | `X-Forwarded-For` | 经可信代理后读取客户端 IP 的请求头（如 `CF-Connecting-IP`） |
| `debug_mode`          | `false`           | 是否启用调试分析 API（在 `/api/debug` 下，需重启生效）      |

#### CORS（`server.cors_origins`）

控制允许跨域访问本服务的浏览器 `Origin`（会话 Cookie 响应会带 `Access-Control-Allow-Credentials`）。

| 取值                      | 效果                                                         |
|---------------------------|--------------------------------------------------------------|
| *（空）*                  | 仅允许主机名匹配 `server.domains` 的 Origin（任意协议/端口） |
| `*.pkg.one`               | 根域 `pkg.one` 及其所有子域（如 `mvnc.pkg.one`）             |
| `https://app.example.com` | 精确匹配完整 Origin                                          |
| `partner.example.com`     | 该主机名下的任意协议/端口                                    |
| `*`                       | 允许任意来源                                                 |

旧配置中的单数形式 `domain: example.com` 仍可加载，并会迁移为 `domains: [example.com]`。

修改 `host`、`port` 或 TLS 相关配置后，需要重启进程。

### `database`

存储账户 Token 与浏览器 Session 的数据库连接参数：

| 键                      | 默认       | 说明                                    |
|-------------------------|------------|-----------------------------------------|
| `enabled`               | `true`     | 是否启用内嵌/外部数据库持久化           |
| `driver`                | `sqlite3`  | 数据库驱动名称（`sqlite3` 或 `mysql`）  |
| `dsn`                   | `renop.db` | 数据库连接串（SQLite 路径或 MySQL DSN） |
| `max_open_conns`        | `25`       | 最大打开连接数                          |
| `max_idle_conns`        | `25`       | 最大空闲连接数                          |
| `conn_max_lifetime_sec` | `300`      | 连接最大复用生存时间（秒）              |

### `frontend`

嵌入式仓库浏览器的品牌相关字段：

| 键                     | 说明                           |
|------------------------|--------------------------------|
| `id`                   | 前端 / 站点标识                |
| `title`                | 页面标题                       |
| `description`          | 简短描述                       |
| `organization_website` | 组织或产品 URL                 |
| `organization_logo`    | 标志路径（如 `/svg/logo.svg`） |
| `background_url`       | 可选背景图 URL                 |
| `icp_license`          | 可选备案或合规说明文字         |

### `updater`

| 键        | 默认      | 说明                                 |
|-----------|-----------|--------------------------------------|
| `channel` | `release` | `release` 或 `nightly`               |
| `mode`    | `manual`  | 更新应用方式（例如在界面中手动安装） |

官网[下载](/download)页与实例内更新使用同一类 stable / nightly 发行源。

## 管理界面

具备 **manager** / **admin** 权限的账户可在「设置」与「仓库」中修改大部分配置项。部分配置在写入文件后需要重载或重启进程（参见各配置域说明）。

## 存储

- **本地磁盘**：使用 `storage_path`（默认方式）
- **S3 兼容**对象存储：在 `repositories.yaml` 中按仓库配置

上传时可生成 MD5 / SHA-1 / SHA-256 / SHA-512 旁路校验文件。

可见性、镜像与 S3 字段说明见 [仓库与镜像](./repositories.md)。

## 相关

- [快速开始](../getting-started/quickstart.md)
- [Maven 客户端](../getting-started/maven-client.md)
- [API 索引](../api/README.md)
