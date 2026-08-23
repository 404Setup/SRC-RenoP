---
title: Обратный прокси
order: 2
category: Развертывание
description: Настройка Nginx и Caddy, завершение TLS и потоковая передача
---

# Обратный прокси (Nginx)

```nginx
location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_request_buffering off;
    proxy_buffering off;
}
```
