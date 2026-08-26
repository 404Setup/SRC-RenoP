---
title: Reverse proxy
order: 2
category: Déploiement
description: Nginx et Caddy avec terminaison TLS, streaming et IP cliente fiable
---

# Reverse proxy

En production, placez RenoP derrière Nginx, Caddy ou un load balancer pour TLS, routage et protection réseau. Le proxy
doit diffuser les gros uploads et blobs sans les charger intégralement en mémoire ou sur disque.

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

Conservez les en-têtes Docker `Location`, `Range`, `Content-Range` et `Docker-Upload-UUID`. N’ajoutez pas de limite de
corps plus faible que les limites RenoP si les clients publient de gros artefacts.

## 2. Caddy

```caddy
renop.example.com {
    reverse_proxy 127.0.0.1:3000 {
        flush_interval -1
    }
}
```

Caddy gère automatiquement TLS. `flush_interval -1` évite de retenir les réponses diffusées.

## 3. Confiance RenoP

Renseignez les hôtes publics et uniquement les CIDR de proxys que vous contrôlez :

```yaml
server:
  domains:
    - "renop.example.com"
  trusted_proxies:
    - "127.0.0.1"
    - "10.0.0.0/8"
  cdn_ip_header: "X-Forwarded-For" # or "CF-Connecting-IP" for Cloudflare
```

RenoP ignore l’en-tête IP si le pair direct n’est pas approuvé. Une configuration trop large permettrait à un client de
falsifier l’adresse utilisée par l’audit et la limitation de débit.
