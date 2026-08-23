---
title: Proxy inverse
order: 2
category: Déploiement
description: Configuration Nginx et Caddy avec terminaison TLS et streaming
---

# Proxy inverse (Nginx)

```nginx
location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_request_buffering off;
    proxy_buffering off;
}
```
