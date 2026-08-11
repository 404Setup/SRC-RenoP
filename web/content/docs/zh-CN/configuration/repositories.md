---
title: 仓库与镜像
order: 2
category: 配置
description: repositories.yaml — 可见性、镜像与 S3
---

# 仓库与镜像

文件：`repositories.yaml`（可用 `RENOP_REPOSITORIES` 覆盖）。

默认仓库：

| 名称        | 用途                    |
|-------------|-------------------------|
| `releases`  | 正式版（一般是 PUBLIC） |
| `snapshots` | 快照（一般是 PUBLIC）   |
| `private`   | 私有（PRIVATE）         |

## 仓库字段

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC          # PUBLIC | HIDDEN | PRIVATE
    allow_redeployment: false
    mirrors: [ ]
    s3:
      enabled: false
      endpoint: ""
      bucket: ""
      key_prefix: ""
      region: auto
      access_key_id: ""
      secret_access_key: ""
      force_path_style: true
      redirect_downloads: false
```

| 字段                 | 说明                                                                               |
|----------------------|------------------------------------------------------------------------------------|
| `name`               | 仓库 ID（路径：`http://host:port/{name}/…`）                                       |
| `visibility`         | `PUBLIC` 匿名可读；`HIDDEN` 限制列表；`PRIVATE` 需读权限                           |
| `allow_redeployment` | 是否允许覆盖已有制品路径（默认：releases/private 为 `false`，snapshots 为 `true`） |
| `mirrors`            | 上游 Maven 代理（可选）                                                            |
| `s3`                 | 该仓库的 S3 兼容后端（可选）                                                       |

目录布局为标准 Maven：`group/artifact/version/file`。

## 镜像

本地没有时从上游拉，并可缓存。

| 字段                                 | 说明                                              |
|--------------------------------------|---------------------------------------------------|
| `name`                               | 名称                                              |
| `url`                                | 上游基址                                          |
| `persist`                            | 是否持久化缓存制品                                |
| `cache_ttl_secs`                     | 正缓存 TTL（秒）                                  |
| `negative_cache`                     | 是否缓存「未找到」                                |
| `timeout_secs`                       | 上游超时                                          |
| `authorization`                      | 可选凭据（`method` / `login` / `password`）       |
| `allow_artifacts` / `deny_artifacts` | 按 `group` 或 `group:artifact` 过滤（勿同时启用） |

## 可见性与权限

| 可见性  | 匿名读 | 说明                                                   |
|---------|--------|--------------------------------------------------------|
| PUBLIC  | 是     | 公开可读                                               |
| HIDDEN  | 受限   | 根列表等需额外角色                                     |
| PRIVATE | 否     | 需要 `canview` / `allview` / `proview`、写权限或管理员 |

写入始终需要 `canupdate`（或管理员）。详见 [认证](../api/authentication.md)。

## S3 兼容存储

`s3.enabled: true` 时，该仓库制品写入指定 bucket。常见字段：`endpoint`、`bucket`、`key_prefix`、`region`、密钥、`force_path_style`（MinIO 常用）、`redirect_downloads`。

`key_prefix` 用于设置 bucket 内的对象键前缀。留空时沿用旧版对象布局。已有制品的仓库添加或修改前缀前，必须先将现有对象迁移到新前缀；RenoP 不会自动迁移。

## 相关

- [配置概览](./overview.md)
- [存储 API](../api/storage.md)
- [Maven 客户端](../getting-started/maven-client.md)
