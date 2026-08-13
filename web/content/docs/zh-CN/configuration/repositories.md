---
title: 仓库与镜像
order: 2
category: 配置
description: repositories.yaml — 可见性、镜像与 S3
---

# 仓库与镜像

文件：`repositories.yaml`（可通过环境变量 `RENOP_REPOSITORIES` 覆盖）。

默认仓库：

| 名称        | 用途                       |
|-------------|----------------------------|
| `releases`  | 正式版本（一般为 PUBLIC）  |
| `snapshots` | 快照版本（一般为 PUBLIC）  |
| `private`   | 私有仓库（一般为 PRIVATE） |

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

| 字段                 | 说明                                                                                 |
|----------------------|--------------------------------------------------------------------------------------|
| `name`               | 仓库 ID（访问路径：`http://host:port/{name}/…`）                                    |
| `visibility`         | `PUBLIC` 匿名可读；`HIDDEN` 限制列表展示；`PRIVATE` 需要读取权限                    |
| `allow_redeployment` | 是否允许覆盖已有制品路径（默认：releases/private 为 `false`，snapshots 为 `true`） |
| `mirrors`            | 上游 Maven 镜像代理列表（可选）                                                      |
| `s3`                 | 该仓库的 S3 兼容对象存储后端配置（可选）                                             |

目录布局遵循标准 Maven 仓库结构：`group/artifact/version/file`。

## 镜像配置

当本地不存在请求的制品时，将从配置的上游镜像拉取，并可缓存到本地。

| 字段                                 | 说明                                                       |
|--------------------------------------|------------------------------------------------------------|
| `name`                               | 镜像名称                                                   |
| `url`                                | 上游仓库基址                                               |
| `persist`                            | 是否持久化缓存的制品                                       |
| `cache_ttl_secs`                     | 正向缓存 TTL（秒）                                         |
| `negative_cache`                     | 是否缓存「未找到」响应                                     |
| `timeout_secs`                       | 上游请求超时时间                                           |
| `authorization`                      | 可选的上游认证凭据（包含 `method`、`login`、`password`）   |
| `allow_artifacts`/`deny_artifacts`   | 按 `group` 或 `group:artifact` 进行过滤（二者不可同时启用）|

## 可见性与权限控制

| 可见性  | 匿名读取 | 说明                                                       |
|---------|----------|------------------------------------------------------------|
| PUBLIC  | 是       | 公开可读，无需认证                                         |
| HIDDEN  | 受限     | 仓库列表等功能需要额外角色权限                             |
| PRIVATE | 否       | 需要 `canview`/`allview`/`proview`、写入权限或管理员权限   |

写入操作始终需要 `canupdate` 权限（或管理员权限）。详见 [认证文档](../api/authentication.md)。

## S3 兼容存储配置

当 `s3.enabled: true` 时，该仓库的制品将写入指定的 S3 兼容存储桶（bucket）。常见配置字段包括：`endpoint`、`bucket`、`key_prefix`、`region`、访问密钥、`force_path_style`（MinIO 等常用）、`redirect_downloads`。

`key_prefix` 用于设置对象存储桶内的对象键前缀。留空时将沿用旧版对象键布局。对于已有制品的仓库，在添加或修改前缀之前，必须先将现有对象手动迁移到新前缀路径下，RenoP 不会自动执行对象迁移。

## 相关文档

- [配置概览](./overview.md)
- [存储 API](../api/storage.md)
- [Maven 客户端配置](../getting-started/maven-client.md)
