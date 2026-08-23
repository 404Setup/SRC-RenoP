---
title: 限流与防御机制
order: 12
category: API 接口
description: 速率限制规则、异常探测与 IP 防护策略说明
---

# 限流与防御机制

为了防止恶意爬虫、扫描探测以及对认证接口的暴力破解，RenoP 内置了多层速率限制与异常行为防御机制。

## 1. 匿名请求限流

对于未提供认证凭据的客户端 IP，系统根据滑动窗口令牌桶算法执行速率限制：

- **公共制品拉取**：允许高频拉取，设定了合理的单 IP 每秒最大请求上限。
- **元数据与搜索接口**：频率限制较为严格，超出后返回 `429 Too Many Requests`。

## 2. 连续认证失败与防爆破拦截

- 当同一 IP 连续多次尝试登录失败，或向私有仓库持续发送无效凭据触发 `401 Unauthorized` / `403 Forbidden` 时，系统会自动将该
  IP 判定为异常来源。
- 触发封禁后，该 IP 在封禁冷却期内发起的所有请求将被直接拒绝并返回 `403 Forbidden`。
- 封禁时长随连续违规次数呈阶梯式延长。

## 3. 并发连接控制 (`max_active_requests`)

在 `config.yaml` 中配置 `server.max_active_requests`（默认 512）：

- 当服务器当前并发处理的 HTTP 请求总数达到该上限时，新到达的请求将被快速拒绝并返回 `503 Service Unavailable`
  ，防止瞬时峰值流量耗尽系统资源。

## 4. 可信代理配置

如果 RenoP 前置了 Nginx、Caddy 或 CDN（如 Cloudflare），请务必在 `config.yaml` 中正确配置 `server.trusted_proxies` 与
`server.cdn_ip_header`，确保限流机制基于客户端真实 IP 进行计算，避免误封反向代理节点的网关 IP。
