---
title: 缓存模式
order: 5
category: 配置
description: 内存、Redis 和 Valkey 缓存及凭据失效机制
---

# 缓存模式

RenoP 默认使用内存缓存。管理员可以在服务设置中选择 Redis 或 Valkey。
缓存模式的更改将在重启后生效。

## 配置

```yaml
cache:
  mode: memory
  address: localhost:6379
  username: ""
  password: ""
  database: 0
  tls: false
  timeout_ms: 1000
```

`mode` 支持 `memory`、`redis` 和 `valkey`。`address` 使用 `host:port` 格式，IPv6 主机需要加方括号。
用户名可选，支持密码认证和 TLS。TLS 会验证服务器证书。
数据库编号范围为 0–65535，并且必须在所选缓存服务中存在。
`timeout_ms` 范围为 10–10000 毫秒，适用于连接、连接池等待、读取和写入。
连接池最多使用八个连接。配置外部模式时，启动需要能够连接外部缓存。

## 缓存数据

所选模式适用于制品元数据内容、解析后的 Maven 元数据、认证结果、数据库账号、
会话、资料和身份查询、存储库缓存策略、上游 Docker 令牌及 SPA 静态资源内容。
生成的 HTML 页面也使用所选模式。数据库存储、活动会话、锁、连接和工作队列
保留原有的归属及持久化方式。

外部缓存值使用进程独立密钥加密并验证完整性。缓存键不包含凭据或请求路径。
本地索引只保留容量淘汰和定向失效所需的字段。
撤销凭据时，即使远端删除失败，也会丢弃本地引用；
在失效前开始的认证查询无法重新填充缓存。

原有缓存容量和逻辑有效期仍然生效。文件内容容量继续由
`server.file_cache_size_mb` 控制。每个外部缓存值最大 2 MiB，最长保留 24 小时；
文件、策略及 SPA 缓存的有效期为一小时。请在缓存服务器上配置总内存上限及淘汰策略。
RenoP 不会修改共享缓存服务器的设置。

缓存重启、淘汰、缺失、密文校验失败或运行期间连接失败时，会回退到数据库、存储或生成逻辑。
连接失败后会绕过共享缓存五秒，再次尝试连接。
RenoP 重启会使用新的命名空间和密钥，旧值按有效期清除。缓存不作为权威数据来源。

## 管理 API

```http
GET /api/settings/cache
PUT /api/settings/cache
POST /api/settings/cache/test
```

这些接口需要系统管理员或获准使用 `admin:settings` 的令牌。
GET 和 PUT 响应不会返回已保存的密码。`password_configured` 表示是否已配置密码；
`restart_required: true` 表示修改需要重启 RenoP。密码留空会保留原密码；
`clear_password: true` 会移除原密码。

PUT 接受上方展示的字段。测试接口接受同样的配置，密码省略时保留已有密码，
仅检查连接，不保存配置。请求体最大 8 KiB。
稳定错误码包括 `cache_settings_invalid`、`cache_settings_save_failed` 和 `cache_connection_failed`。

缓存账号需要对 `renop:*` 执行 PING、SET、GETRANGE、DEL 的权限，以及服务器要求的认证和数据库选择命令权限。
这些接口不配置 Sentinel 或集群发现。
