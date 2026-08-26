---
title: Outbound Proxy
order: 4
category: Configuration
description: Named HTTP, HTTPS, and SOCKS5 proxies and per-mirror routing
---

# Outbound Proxy Configuration

Configure outbound proxies when RenoP must reach Maven Central, crates.io, Docker registries, GitHub, GitLab, or GPG
key servers through a controlled egress path. The process shares bounded HTTP transports per routing policy.

## Configuration schema

```yaml
proxy:
  selected: "corp_http"
  proxies:
    - name: "corp_http"
      url: "http://10.0.0.1:8080"
      username: "proxy-user"
      password: "proxy-password"
    - name: "socks_proxy"
      url: "socks5://10.0.0.2:1080"
      username: ""
      password: ""
```

`selected` is the global default. An empty value means direct access. At most 16 named proxies are accepted. Names must
be unique. URLs support `http`, `https`, or `socks5`, must include an appropriate host and port, and must not contain
credentials, paths, queries, or fragments. Put credentials only in `username` and `password`.

## Routing behavior

| Selector | Result |
|:---------|:-------|
| `""` | Inherit the global `proxy.selected` value |
| `direct` | Bypass every proxy |
| A proxy name | Use that exact configured proxy |

Changing the selected proxy or credentials invalidates the relevant pooled clients. An unknown selector is rejected
instead of silently falling back to direct access.

## Per-mirror selection

Each repository mirror may override the global route with its `proxy` field:

```yaml
repositories:
  releases:
    name: releases
    format: maven
    mirrors:
      - name: "maven-central"
        url: "https://repo1.maven.org/maven2"
        proxy: "corp_http"
      - name: "internal"
        url: "https://mirror.internal/maven"
        proxy: "direct"
      - name: "default-route"
        url: "https://plugins.gradle.org/m2"
        proxy: ""
```

Use `direct` for internal services that must never traverse the global proxy. Keep secrets out of mirror URLs and logs.
