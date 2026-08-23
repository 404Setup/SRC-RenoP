---
title: 出站代理配置
order: 4
category: 配置
description: HTTP、HTTPS 与 SOCKS5 出站代理配置与按镜像路由
---

# 出站代理配置

当 RenoP 部署在内网或受限网络环境中，且需要拉取公网上游仓库（如 Maven Central、crates.io 或 Docker Hub）时，可以通过配置出站代理建立连接。

相关配置位于 `config.yaml` 的 `proxy` 节点下。

## 配置结构

```yaml
proxy:
  selected: "corp_http"   # 默认生效的代理名称，留空表示直连
  proxies:
    corp_http:
      url: "http://10.0.0.1:8080"
    socks_proxy:
      url: "socks5://user:pass@10.0.0.2:1080"
    https_proxy:
      url: "https://proxy.internal:8443"
```

## 字段说明

| 字段名               | 类型   | 说明                                                                                          |
|:---------------------|:-------|:----------------------------------------------------------------------------------------------|
| `selected`           | string | 全局默认代理名称。留空（`""`）表示所有未特别指定代理的镜像均采用直连。                        |
| `proxies`            | map    | 命名代理字典。Key 为自定义代理标识，Value 为代理详细配置。                                    |
| `proxies.<name>.url` | string | 代理服务器 URL。支持 `http://`、`https://` 与 `socks5://` 协议，可在 URL 中包含认证账号密码。 |

## 按镜像指定代理

在 `repositories.yaml` 中配置具体镜像时，可以通过 `proxy` 字段单独指定该镜像所使用的代理策略：

```yaml
repositories:
  releases:
    name: releases
    mirrors:
      - name: "upstream-maven"
        url: "https://repo1.maven.org/maven2"
        proxy: "corp_http"     # 使用名为 corp_http 的代理

      - name: "internal-mirror"
        url: "http://mirror.internal/maven"
        proxy: "direct"        # 强制直连，绕过全局默认代理

      - name: "default-mirror"
        url: "https://plugins.gradle.org/m2"
        proxy: ""              # 留空表示跟随全局 proxy.selected 设置
```

| `proxy` 取值 | 行为                                                  |
|:-------------|:------------------------------------------------------|
| `""`（留空） | 跟随 `config.yaml` 中 `proxy.selected` 的全局代理设置 |
| `"direct"`   | 强制直连，不走任何代理                                |
| `"代理名称"` | 强制使用 `config.yaml` 中定义的指定代理               |
