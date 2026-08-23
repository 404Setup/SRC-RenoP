---
title: 配置概览
order: 1
category: 配置
description: config.yaml 配置项、服务设置与环境变量
---

# 配置概览

RenoP 的主配置文件为 `config.yaml`。启动时程序会从工作目录中读取该文件，也可以通过环境变量 `RENOP_CONFIG` 指定其他路径。

## 配置文件列表

| 文件名              | 环境变量             | 说明                                                  |
|:--------------------|:---------------------|:------------------------------------------------------|
| `config.yaml`       | `RENOP_CONFIG`       | 服务端口、TLS、数据库连接、存储路径、代理与更新器配置 |
| `repositories.yaml` | `RENOP_REPOSITORIES` | 仓库定义、可见性、上游镜像与 S3 存储桶配置            |
| `tokens.yaml`       | `RENOP_TOKENS`       | 初始用户与静态 Token（启动后会自动导入数据库）        |
| `index.json`        | `RENOP_INDEX`        | 制品搜索索引缓存                                      |
| `sessions.bin`      | `RENOP_SESSIONS`     | 浏览器会话数据缓存                                    |

## `config.yaml` 详细配置项

### 基础存储与文档预览

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
```

| 配置项                   | 默认值    | 说明                                               |
|:-------------------------|:----------|:---------------------------------------------------|
| `storage_path`           | `storage` | 本地制品存储根目录                                 |
| `enable_javadoc_preview` | `true`    | 是否启用 Javadoc 在线解压与预览功能                |
| `javadoc_extract_path`   | `""`      | Javadoc 解压临时目录（留空则使用系统默认缓存目录） |
| `max_javadoc_size_mb`    | `48`      | 单个 Javadoc JAR 解压大小上限（MB）                |

### `server` 服务端网络与安全

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains:
    - "localhost"
  cors_origins: [ ]
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: [ ]
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
  gpg:
    key_servers:
      - "https://keys.openpgp.org"
      - "https://keyserver.ubuntu.com"
```

| 配置项                | 默认值             | 说明                                                                            |
|:----------------------|:-------------------|:--------------------------------------------------------------------------------|
| `host`                | `0.0.0.0`          | 监听 IP 地址                                                                    |
| `port`                | `3000`             | 监听端口                                                                        |
| `ssl_enabled`         | `false`            | 是否开启内置 TLS/HTTPS                                                          |
| `ssl_cert_path`       | `""`               | TLS 证书文件路径（`.crt` 或 `.pem`）                                            |
| `ssl_key_path`        | `""`               | TLS 私钥文件路径（`.key`）                                                      |
| `domains`             | `["localhost"]`    | 实例对外访问的主机名列表（用于生成下载链接与默认 CORS）                         |
| `cors_origins`        | `[]`               | 允许跨域访问的 Origin 列表（空表示仅允许 `domains` 中的域名，`*` 表示允许全部） |
| `enable_compression`  | `false`            | 是否开启 HTTP 响应 gzip/brotli 压缩                                             |
| `file_cache_size_mb`  | `16`               | 静态小文件与元数据内存缓存大小（MB）                                            |
| `max_active_requests` | `512`              | 最大并发处理请求数（超出返回 503）                                              |
| `trusted_proxies`     | `[]`               | 信任的反向代理 IP 或 CIDR 列表（环回地址默认信任）                              |
| `cdn_ip_header`       | `X-Forwarded-For`  | 从受信任代理获取真实客户端 IP 的 HTTP 标头名称                                  |
| `debug_mode`          | `false`            | 是否开放 `/api/debug` 性能分析与 pprof 端点（修改后需重启）                     |
| `gpg.key_servers`     | 默认公共密钥服务器 | 用于验证上传制品签名的 OpenPGP 密钥服务器列表                                   |

> **注意**：修改 `host`、`port` 或 TLS 相关配置后，需重启 RenoP 进程生效。

### `database` 数据库连接

```yaml
database:
  enabled: true
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

支持 `sqlite3`、`mysql` 与 `postgres`。详细配置与参数说明请参阅 [数据库配置](./database.md)。

### `proxy` 出站代理

```yaml
proxy:
  selected: ""
  proxies:
    corp_proxy:
      url: "http://proxy.internal:8080"
```

用于配置 RenoP 向外拉取上游镜像时的 HTTP/HTTPS/SOCKS5 代理。详细说明请参阅 [出站代理配置](./outbound-proxy.md)。

### `frontend` 界面定制

```yaml
frontend:
  id: "renop"
  title: "RenoP Package Registry"
  description: "Self-hosted package repository"
  organization_website: ""
  organization_logo: "/svg/logo.svg"
  background_url: ""
  icp_license: ""
  public_security_filing: ""
  legal_notice_url: ""
```

用于自定义 Web 管理界面的标题、Logo、备案号及组织链接。

### `updater` 在线更新

```yaml
updater:
  channel: "release"    # release 或 nightly
  mode: "manual"        # manual（手动在界面点击更新）
```

## 相关文档

- [仓库与镜像配置](./repositories.md)
- [数据库配置](./database.md)
- [出站代理配置](./outbound-proxy.md)
