---
title: Outbound Proxy
order: 4
category: Configuration
description: HTTP, HTTPS, and SOCKS5 outbound proxy configuration and routing
---

# Outbound Proxy Configuration

When RenoP is deployed inside an isolated corporate network and needs to reach public upstream registries (such as Maven
Central, crates.io, or Docker Hub), configure outbound proxies under the `proxy` block in `config.yaml`.

## Configuration Schema

```yaml
proxy:
  selected: "corp_http"   # Default active proxy name; empty for direct connection
  proxies:
    corp_http:
      url: "http://10.0.0.1:8080"
    socks_proxy:
      url: "socks5://user:pass@10.0.0.2:1080"
    https_proxy:
      url: "https://proxy.internal:8443"
```

## Parameter Reference

| Parameter            | Type   | Description                                                                                      |
|:---------------------|:-------|:-------------------------------------------------------------------------------------------------|
| `selected`           | string | Global default proxy identifier. Leave empty (`""`) for direct connection.                       |
| `proxies`            | map    | Named dictionary of proxy servers.                                                               |
| `proxies.<name>.url` | string | Proxy URL supporting `http://`, `https://`, and `socks5://` protocols with embedded credentials. |

## Per-Mirror Proxy Routing

In `repositories.yaml`, individual mirrors can specify their own proxy behavior via the `proxy` field:

```yaml
repositories:
  releases:
    name: releases
    mirrors:
      - name: "upstream-maven"
        url: "https://repo1.maven.org/maven2"
        proxy: "corp_http"     # Routes via named proxy corp_http

      - name: "internal-mirror"
        url: "http://mirror.internal/maven"
        proxy: "direct"        # Forces direct connection, bypassing global proxy

      - name: "default-mirror"
        url: "https://plugins.gradle.org/m2"
        proxy: ""              # Inherits global proxy.selected setting
```
