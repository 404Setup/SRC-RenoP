---
title: Обратный прокси
order: 2
category: Развёртывание
description: Nginx и Caddy с TLS, streaming и доверенным IP клиента
---

# Обратный прокси

В production размещайте RenoP за Nginx, Caddy или load balancer для TLS, маршрутизации и сетевой защиты. Прокси должен
потоково передавать большие uploads и blobs без полного буфера в памяти или на диске.

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

Сохраняйте Docker-заголовки `Location`, `Range`, `Content-Range` и `Docker-Upload-UUID`. Для крупных артефактов не
задавайте на прокси body limit ниже ограничения RenoP.

## Caddy

```caddy
renop.example.com {
    reverse_proxy 127.0.0.1:3000 {
        flush_interval -1
    }
}
```

Caddy автоматически управляет TLS. `flush_interval -1` не задерживает streaming responses.

### Автоматическая настройка

Запустите installer из каталога RenoP. Он ищет Caddyfile в стандартных расположениях, проверяет новый site бинарным
файлом Caddy, транзакционно обновляет оба файла и перезагружает Caddy. В `config.yaml` синхронизируются публичный hostname,
loopback listener и передача TLS под управление Caddy.

```bash
./renop --install-caddy --hostname renop.example.com

# Explicit paths or offline preparation
./renop --install-caddy --hostname renop.example.com \
  --caddyfile /etc/caddy/Caddyfile \
  --config /opt/renop/config.yaml \
  --skip-reload
```

После успешной команды перезапустите RenoP. В обычном режиме не используйте `--skip-reload`; offline mode явно
пропускает проверку и reload, если бинарный файл Caddy недоступен.

## Настройка доверия RenoP

Укажите публичные host и только CIDR прокси, которыми вы управляете:

```yaml
server:
  domains:
    - "renop.example.com"
  trusted_proxies:
    - "127.0.0.1"
    - "10.0.0.0/8"
  cdn_ip_header: "X-Forwarded-For" # or "CF-Connecting-IP" for Cloudflare
```

RenoP игнорирует forwarded IP, если прямой peer не доверен. Слишком широкая сеть доверия позволит подделать адрес,
используемый аудитом и rate limiting.
