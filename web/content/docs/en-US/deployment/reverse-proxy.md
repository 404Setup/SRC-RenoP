---
title: Reverse Proxy Setup
order: 2
category: Deployment
description: Configuring Nginx and Caddy reverse proxies with TLS termination and streaming
---

# Reverse Proxy Setup

In production deployments, RenoP is commonly placed behind Nginx, Caddy, or a cloud load balancer for TLS termination,
traffic routing, and DDoS protection.

## 1. Nginx Configuration

For streaming large artifacts and Docker image layers, adjust buffer settings and body limits:

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

## 2. Caddy Configuration

```caddy
renop.example.com {
    reverse_proxy 127.0.0.1:3000 {
        flush_interval -1
    }
}
```

## 3. RenoP Trust Configuration

To preserve real client IPs for rate limiting and audit logs, configure trusted proxies in `config.yaml`:

```yaml
server:
  domains:
    - "renop.example.com"
  trusted_proxies:
    - "127.0.0.1"
    - "10.0.0.0/8"
  cdn_ip_header: "X-Forwarded-For" # or "CF-Connecting-IP" for Cloudflare
```
