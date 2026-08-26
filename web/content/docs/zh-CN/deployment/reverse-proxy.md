---
title: 反向代理
order: 2
category: 部署
description: 使用 Nginx 与 Caddy 进行 TLS 终止、流式传输与可信客户端 IP 转发
---

# 反向代理

生产环境通常将 RenoP 部署在 Nginx、Caddy 或负载均衡器后方，以提供 TLS、路由与网络防护。代理必须能够
流式转发大型上传与 Blob，不应将完整正文缓冲到内存或磁盘。

## Nginx

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

    # Allow large artifact uploads
    client_max_body_size 0;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;

        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Disable buffering for real-time streaming
        proxy_request_buffering off;
        proxy_buffering off;

        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

必须保留 Docker 使用的 `Location`、`Range`、`Content-Range` 与 `Docker-Upload-UUID` 请求头。发布大型制品时，
代理的正文上限不可低于 RenoP 限制。

## Caddy

```caddy
renop.example.com {
    reverse_proxy 127.0.0.1:3000 {
        flush_interval -1
    }
}
```

Caddy 自动管理 TLS；`flush_interval -1` 可避免延迟流式响应。

## RenoP 信任配置

只配置公开主机名及由自己控制的代理 CIDR：

```yaml
server:
  domains:
    - "renop.example.com"
  trusted_proxies:
    - "127.0.0.1"
    - "10.0.0.0/8"
  cdn_ip_header: "X-Forwarded-For" # or "CF-Connecting-IP" for Cloudflare
```

直接连接来源不受信任时，RenoP 会忽略转发 IP 请求头。过宽的信任范围会允许客户端伪造行为日志与速率限制所
使用的 IP。
