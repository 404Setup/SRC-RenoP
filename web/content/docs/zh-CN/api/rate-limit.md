---
title: 速率限制与防护
order: 12
category: API 接口
description: 请求速率限制、异常检测与 IP 防护策略
---

# 速率限制与防护

RenoP 组合使用多层速率限制与异常检测，降低暴力破解、拒绝服务和过量自动抓取带来的风险。

## 匿名请求限制

未认证请求按客户端 IP 使用滑动窗口与令牌桶进行控制：

- 公开制品下载使用较宽松的上限；
- 搜索与元数据请求使用更严格的上限，超出后返回 `429 Too Many Requests`。

## 连续认证失败与 IP 封禁

- 登录接口或私有资源连续产生 `401 Unauthorized` 或 `403 Forbidden` 时，会被判定为异常行为；
- 对应 IP 将被临时封禁并返回 `403 Forbidden`，重复触发会延长封禁时间。

## 并发上限 (`max_active_requests`)

在 `config.yaml` 中配置 `server.max_active_requests`，默认值为 512。

- 活跃请求达到上限后，新请求返回 `503 Service Unavailable`。

## 可信代理

部署在反向代理或 CDN 后方时，应在 `config.yaml` 中配置 `server.trusted_proxies` 与
`server.cdn_ip_header`。RenoP 仅使用来自可信来源且经过校验的真实客户端 IP 执行速率限制。
