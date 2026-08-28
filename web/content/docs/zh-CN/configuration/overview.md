---
title: 配置概览
order: 1
category: 配置
description: 配置文件、服务设置、存储、代理、品牌与更新策略
---

# 配置概览

RenoP 默认从工作目录读取 `config.yaml`，可通过 `RENOP_CONFIG` 覆盖路径。管理员界面写入设置时使用相同的
校验结构与私有文件权限。

## 配置文件

| 文件 | 覆盖变量 | 用途 |
|:-----|:---------|:-----|
| `config.yaml` | `RENOP_CONFIG` | 服务、数据库、文档预览、代理、前端、审计与更新器 |
| `repositories.yaml` | `RENOP_REPOSITORIES` | 仓库引擎、可见性、镜像、Maven 策略与 S3 |
| `index.json` | `RENOP_INDEX` | 持久化文件索引快照，必要时可从存储重建 |

账号、API Token、会话、团队、行为日志与消息均存储在数据库中，不通过 YAML 配置。配置文件和仓库文件可能
包含凭据，应只允许服务账号读取。

## `config.yaml` 结构

### 存储与文档预览

```yaml
storage_path: "storage"
enable_javadoc_preview: true
javadoc_extract_path: ""
max_javadoc_size_mb: 48
enable_cargodoc_preview: true
cargodoc_extract_path: ""
max_cargodoc_size_mb: 128
```

提取路径留空时使用平台缓存目录。通过 `/javadoc` 或 `/cargodoc` 暴露内容前，会校验归档路径与大小限制。

### `server` 网络与安全

```yaml
server:
  host: "0.0.0.0"
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains: ["localhost"]
  cors_origins: []
  enable_compression: false
  file_cache_size_mb: 16
  max_active_requests: 512
  trusted_proxies: []
  cdn_ip_header: "X-Forwarded-For"
  debug_mode: false
  gpg:
    key_servers: ["https://keys.openpgp.org", "https://keyserver.ubuntu.com"]
```

`domains` 提供公开主机名与默认 CORS 主机。`cors_origins` 可增加精确 Origin、主机或通配主机，`*` 表示允许
全部 Origin。只有直接连接来源匹配 `trusted_proxies` 时才信任转发客户端 IP 请求头。主机、端口、TLS、压缩、
调试模式及部分缓存设置变更要求重启。

GitHub OAuth 同样存储在 `server.github_oauth` 下；应通过界面配置 Client ID 与只写 Secret。

### `database` 数据库连接

```yaml
database:
  driver: "sqlite3"
  dsn: "renop.db"
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300
```

支持 `sqlite3`（或 `sqlite`）、`mysql`、`postgres` 与原生 `clickhouse`。详见[数据库配置](./database.md)。

### `proxy` 出站路由

```yaml
proxy:
  selected: ""
  proxies:
    - name: "corp_proxy"
      url: "http://proxy.internal:8080"
      username: ""
      password: ""
```

最多配置 16 个 HTTP、HTTPS 或 SOCKS5 代理。详见[出站代理配置](./outbound-proxy.md)。

### `frontend` 品牌设置

```yaml
frontend:
  id: "renop"
  title: "RenoP Package Registry"
  description: "Self-hosted package repository"
  organization_website: ""
  organization_logo: "/svg/logo.svg"
  background_url: ""
  font_preset: "system"
  font_url: ""
  icp_license: ""
  public_security_filing: ""
  legal_notice_url: ""
```

品牌 URL 使用前会被校验。背景图必须满足 WebP 格式与大小策略。
`font_preset` 支持 `system`、`inter`、`noto_sans`、`open_sans`、`source_sans` 和 `custom`。预设使用本机
已安装字体；自定义值可使用 WOFF2、WOFF、TTF 文件直链或 Google Fonts CSS URL。资源在后台加载，主要字体
完整可用后才会启用，因此不会阻塞首次渲染。

### `updater` 更新策略

```yaml
updater:
  channel: "release"
  mode: "manual"
```

`channel` 为 `release` 或 `nightly`；`mode` 为 `manual`、`auto_check` 或 `auto_install`。自动检查由进程级调度器
合并执行，结果通过消息中心发送给管理员。
