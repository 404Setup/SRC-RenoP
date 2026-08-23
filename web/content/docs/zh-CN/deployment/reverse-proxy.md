---
title: 反向代理配置
order: 2
category: 运维部署
description: 配置 Nginx 与 Caddy 反向代理、HTTPS 终止与真实 IP 传递
---

# 反向代理配置

在生产环境中，通常将 RenoP 部署在 Nginx、Caddy 或云厂商负载均衡器之后，由反向代理负责 TLS 证书托管、请求路由与流量清洗。

## 1. Nginx 配置示例

对于包含大文件上传与制品流式下载的场景，需要调整 Nginx 的客户端请求体限制与缓冲设置：

```nginx
server {
    listen 80;
    server_name renop.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name renop.example.com;

    ssl_certificate     /etc/ssl/certs/renop.example.com.crt;
    ssl_certificate_key /etc/ssl/private/renop.example.com.key;

    # 允许上传大型制品与 Docker 镜像分块
    client_max_body_size 0;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;

        # 传递真实客户端 IP 与协议
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 禁用请求体缓冲以支持流式上传
        proxy_request_buffering off;
        proxy_buffering off;

        # 调整超时时间以支持大文件传输
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

## 2. Caddy 配置示例

Caddy 支持自动申请并维护 HTTPS 证书：

```caddy
renop.example.com {
    reverse_proxy 127.0.0.1:3000 {
        # 禁用流式缓冲
        flush_interval -1
    }
}
```

## 3. RenoP 端信任反向代理配置

当经过反向代理时，为了让 RenoP 正确识别客户端的真实 IP 并进行合理的限流与审计日志记录，需在 `config.yaml` 中配置受信任代理：

```yaml
server:
  domains:
    - "renop.example.com"
  trusted_proxies:
    - "127.0.0.1"
    - "10.0.0.0/8"
  cdn_ip_header: "X-Forwarded-For" # 若使用 Cloudflare 可设为 "CF-Connecting-IP"
```
