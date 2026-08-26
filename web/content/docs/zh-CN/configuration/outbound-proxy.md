---
title: 出站代理
order: 4
category: 配置
description: 命名 HTTP、HTTPS、SOCKS5 代理与镜像独立路由
---

# 出站代理配置

RenoP 需要通过受控出口访问 Maven Central、crates.io、Docker、GitHub、GitLab 或 GPG 密钥服务器时，应配置
出站代理。进程按照路由策略复用有界 HTTP Transport。

## 配置结构

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

`selected` 是全局默认值，留空表示直接连接。最多配置 16 个名称唯一的代理。URL 支持 `http`、`https` 与
`socks5`，必须包含适用的主机和端口，不可包含凭据、路径、查询参数或片段。凭据只能写入 `username` 和
`password`。

## 路由行为

| 选择器 | 结果 |
|:-------|:-----|
| `""` | 继承全局 `proxy.selected` |
| `direct` | 绕过全部代理 |
| 代理名称 | 使用精确匹配的代理配置 |

修改选中代理或凭据后，相关共享客户端会立即失效。未知代理名称会被拒绝，不会静默回退到直接连接。

## 镜像独立选择

每个存储库镜像可通过 `proxy` 覆盖全局路由：

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

必须绕过全局代理的内部服务使用 `direct`。不得在镜像 URL 或日志中写入密钥。
