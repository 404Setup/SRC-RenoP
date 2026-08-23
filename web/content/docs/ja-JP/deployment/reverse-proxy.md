---
title: リバースプロキシ設定
order: 2
category: デプロイと運用
description: Nginx および Caddy による TLS 終端とストリーミング最適化
---

# リバースプロキシ設定

## Nginx 設定例

```nginx
server {
    listen 443 ssl http2;
    server_name renop.example.com;

    client_max_body_size 0;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        proxy_request_buffering off;
        proxy_buffering off;
    }
}
```
