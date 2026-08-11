---
title: 设置
order: 6
category: API
---

# 设置与仓库配置

前缀：`/api/settings`

读写均需 **manager / admin**。

该前缀下携带结构化数据的请求/响应正文均使用 **`application/x-protobuf`**（见 `proto/api/v1/api.proto`）。空的成功正文仍为
纯文本（`""`）。校验错误仍为简短英文文本。

磁盘位置：

| 内容       | 文件                | 环境变量             |
|------------|---------------------|----------------------|
| 域设置     | `config.yaml`       | `RENOP_CONFIG`       |
| Maven 仓库 | `repositories.yaml` | `RENOP_REPOSITORIES` |

监听器 / TLS 变更需进程重启才能完全生效。

## 索引

### `POST /api/settings/index/rebuild`

请求：protobuf `RebuildIndexRequest`

| 字段   | 类型   | 取值             |
|--------|--------|------------------|
| `mode` | string | `full` \| `diff` |

| mode   | 行为                            |
|--------|---------------------------------|
| `full` | 异步全量重建；清除 Javadoc 缓存 |
| `diff` | 差分重建                        |

其他 → 400（`Invalid mode. Expected 'full' or 'diff'`）。成功：200，空字符串正文。

## 配置域

### `GET /api/settings/domains`

响应：protobuf `SettingsDomainsResponse`

| 字段      | 类型            |
|-----------|-----------------|
| `domains` | repeated string |

典型值：`frontend`、`server`、`storage`、`updater`、`index`。

`index` 当前无可配置字段。

### `GET /api/settings/domain/:name`

响应：该域的 protobuf 消息（Content-Type `application/x-protobuf`）。

**frontend** → `FrontendConfig`

| 字段                     | 类型   |
|--------------------------|--------|
| `id`                     | string |
| `title`                  | string |
| `description`            | string |
| `organization_website`   | string |
| `organization_logo`      | string |
| `background_url`         | string |
| `icp_license`            | string |
| `public_security_filing` | string |
| `legal_notice_url`       | string |

**server** → `ServerConfig`

| 字段                  | 类型            | 说明                   |
|-----------------------|-----------------|------------------------|
| `host`                | string          | 监听 IP 地址           |
| `port`                | uint32          | 监听端口               |
| `ssl_enabled`         | bool            | 是否启用 TLS           |
| `ssl_cert_path`       | string          | TLS 证书文件路径       |
| `ssl_key_path`        | string          | TLS 私钥文件路径       |
| `domains`             | repeated string | 本实例对外域名列表     |
| `enable_compression`  | bool            | 是否启用 HTTP 响应压缩 |
| `file_cache_size_mb`  | uint32          | 文件内存缓存上限（MB） |
| `max_active_requests` | uint32          | 并发请求上限           |
| `trusted_proxies`     | repeated string | 可信代理 CIDR/IP 列表  |
| `cdn_ip_header`       | string          | 客户端 IP 请求头名称   |
| `cors_origins`        | repeated string | CORS 允许来源列表      |
| `debug_mode`          | bool            | 是否启用调试分析 API   |
| `database`            | DatabaseConfig  | 数据库连接配置         |

**DatabaseConfig**：

| 字段                    | 类型   | 说明                                 |
|-------------------------|--------|--------------------------------------|
| `enabled`               | bool   | 是否启用数据库持久化                 |
| `driver`                | string | 数据库驱动（`sqlite3` 或 `mysql`）   |
| `dsn`                   | string | 数据库 DSN/文件路径（如 `renop.db`） |
| `max_open_conns`        | int32  | 最大打开连接数                       |
| `max_idle_conns`        | int32  | 最大空闲连接数                       |
| `conn_max_lifetime_sec` | int32  | 连接最大复用时间（秒）               |

**storage** → `StorageConfig`

| 字段                     | 类型   |
|--------------------------|--------|
| `storage_path`           | string |
| `enable_javadoc_preview` | bool   |
| `javadoc_extract_path`   | string |
| `max_javadoc_size_mb`    | int64  |

**updater** → `UpdaterConfig`

| 字段      | 类型   | 取值                                                         |
|-----------|--------|--------------------------------------------------------------|
| `channel` | string | `release` \| `nightly`                                       |
| `mode`    | string | `manual` \| `auto_check` \| `auto_install` \| `safe_install` |

**index** → 空的 `IndexDomainSettings`

### `PUT /api/settings/domain/:name`

对该域做 **完整替换**。正文与该域 GET 相同的 protobuf 消息。 Proto3 省略字段解码为零值 — 客户端必须发送 完整域配置（UI 始终
POST 完整表单状态）。

成功：200，空字符串。

规则：

- `frontend.background_url`：非空时须可达、公网 IP、WebP、≤ 5 MiB；拒绝私有地址
- `storage.max_javadoc_size_mb`：必须 > 0
- `storage.storage_path`：改到不同路径时，服务器立即对新根全量重建文件索引（并重启 FS 监视器）；清除 Javadoc 缓存
- `updater.channel` / `updater.mode`：必须是允许的枚举值（空无效）
- `index`：无可写内容 → 404

校验失败 → 400 + 简短英文错误文本。

## Maven 仓库

### `GET /api/settings/maven/repositories`

响应：protobuf `MavenRepositoriesResponse`（`map<string, Repository>`）。

| 字段                 | 含义                                               |
|----------------------|----------------------------------------------------|
| `name`               | 仓库名                                             |
| `visibility`         | `PUBLIC` / `HIDDEN` / `PRIVATE`                    |
| `allow_redeployment` | 是否允许覆盖已有制品                               |
| `mirrors[]`          | 上游镜像（url、persist、TTL、auth、allow/deny 等） |
| `s3`                 | 可选 S3 兼容存储                                   |

### `PUT /api/settings/maven/repositories/:name`

创建或 **完整替换**。正文为 protobuf `Repository`。路径 `:name` 优先于正文 `name`。

保留名：`css`、`js`、`svg`、`api`、`javadocs`、`assets`，以及非法字符。

成功：200，空字符串。

### `DELETE /api/settings/maven/repositories/:name`

从配置中移除； **不**删除磁盘上的文件。成功：200，空字符串。
