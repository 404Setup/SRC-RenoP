---
title: リバースプロキシ
order: 2
category: デプロイ
description: Nginx と Caddy の TLS 終端、streaming、信頼済み client IP
---

# リバースプロキシ

本番環境では TLS、routing、network protection のため RenoP を Nginx、Caddy、load balancer の背後に置きます。
大きな upload や blob をメモリまたは disk に全量保持せず stream できる設定にしてください。

## 1. Nginx

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

Docker の `Location`、`Range`、`Content-Range`、`Docker-Upload-UUID` header を保持します。大きな成果物を
公開する場合、proxy 側の body limit を RenoP より小さくしないでください。

## 2. Caddy

```caddy
renop.example.com {
    reverse_proxy 127.0.0.1:3000 {
        flush_interval -1
    }
}
```

Caddy は TLS を自動管理します。`flush_interval -1` は streaming response の遅延を防ぎます。

## 3. RenoP の信頼設定

公開 host と、自分で管理する proxy CIDR だけを設定します。

```yaml
server:
  domains:
    - "renop.example.com"
  trusted_proxies:
    - "127.0.0.1"
    - "10.0.0.0/8"
  cdn_ip_header: "X-Forwarded-For" # or "CF-Connecting-IP" for Cloudflare
```

直接接続元が信頼済みでなければ RenoP は転送 IP header を無視します。広すぎる trust は、audit と rate limit
で使う client IP の偽装を許します。
