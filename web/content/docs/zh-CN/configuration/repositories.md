---
title: 仓库与镜像
order: 2
category: 配置
description: repositories.yaml 仓库定义、可见性、上游镜像与 S3 存储
---

# 仓库与镜像配置

仓库的定义存放在 `repositories.yaml` 中（可通过环境变量 `RENOP_REPOSITORIES` 覆盖路径）。大部分选项也可以在 Web
控制台的「仓库设置」中进行可视化修改。

## 配置文件示例

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: [ ]
    s3:
      enabled: false

  snapshots:
    name: snapshots
    visibility: PUBLIC
    allow_redeployment: true
    require_gpg_signature: false
    mirrors: [ ]
    s3:
      enabled: false

  private:
    name: private
    visibility: PRIVATE
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: [ ]
    s3:
      enabled: false
```

## 仓库基础字段

| 字段名                  | 类型   | 默认值   | 说明                                                     |
|:------------------------|:-------|:---------|:---------------------------------------------------------|
| `name`                  | string | 必填     | 仓库标识与 URL 路径前缀（如 `http://host:3000/{name}/`） |
| `visibility`            | string | `PUBLIC` | 仓库可见性级别，可选 `PUBLIC`、`HIDDEN`、`PRIVATE`       |
| `allow_redeployment`    | bool   | `false`  | 是否允许重新上传并覆盖已存在的同名同版本制品文件         |
| `require_gpg_signature` | bool   | `false`  | 是否强制要求附带 GPG 分离签名并在验签通过前暂存隔离区    |
| `mirrors`               | list   | `[]`     | 上游镜像代理列表                                         |
| `s3`                    | object | `{}`     | 该仓库绑定的 S3 兼容对象存储配置                         |

### 可见性级别说明

- **PUBLIC**：公开可读。匿名用户无需登录即可直接拉取依赖或浏览制品列表。
- **HIDDEN**：受限公开。知晓具体制品路径的用户可以直接拉取，但在公共列表页面中不对未登录用户展示。
- **PRIVATE**：私有仓库。拉取制品、浏览列表与上传均需要提供凭据，且当前用户必须拥有该仓库的查看或写入权限。

## 上游镜像代理配置 (`mirrors`)

当本地仓库中未找到客户端请求的依赖时，RenoP 可向上游镜像仓库代理请求，并可将制品缓存至本地。

```yaml
mirrors:
  - name: "aliyun-maven"
    url: "https://maven.aliyun.com/repository/public"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: [ ]
    deny_artifacts: [ ]
```

| 字段名            | 默认值  | 说明                                                                                  |
|:------------------|:--------|:--------------------------------------------------------------------------------------|
| `name`            | 必填    | 镜像标识名称                                                                          |
| `url`             | 必填    | 上游仓库的基础 URL                                                                    |
| `persist`         | `true`  | 是否将从上游成功拉取的制品持久化保存到本地存储中                                      |
| `cache_ttl_secs`  | `86400` | 镜像拉取制品的缓存有效期（秒）                                                        |
| `negative_cache`  | `true`  | 是否对上游 404 响应开启负缓存（避免短时间内重复发起无效请求）                         |
| `timeout_secs`    | `30`    | 请求上游的超时时间（秒）                                                              |
| `proxy`           | `""`    | 留空使用全局默认代理；设为 `direct` 表示直连；或填写在 `config.yaml` 中配置的代理名称 |
| `allow_artifacts` | `[]`    | 白名单规则列表（如 `com.example` 或 `com.example:lib`），仅代理匹配的坐标             |
| `deny_artifacts`  | `[]`    | 黑名单规则列表，阻止代理匹配的坐标                                                    |

## S3 对象存储配置 (`s3`)

如果需要将特定仓库的数据存储在云端或自建 MinIO 中，可为该仓库启用 S3 配置：

```yaml
s3:
  enabled: true
  endpoint: "https://s3.us-east-1.amazonaws.com"
  bucket: "my-renop-bucket"
  key_prefix: "releases/"
  region: "us-east-1"
  access_key_id: "YOUR_ACCESS_KEY"
  secret_access_key: "YOUR_SECRET_KEY"
  force_path_style: false
  redirect_downloads: false
```

| 字段名               | 默认值  | 说明                                                                          |
|:---------------------|:--------|:------------------------------------------------------------------------------|
| `enabled`            | `false` | 是否启用该仓库的 S3 存储                                                      |
| `endpoint`           | 必填    | S3 API 服务端点 URL（使用 AWS 时可填对应区域端点，自建 MinIO 填实际地址）     |
| `bucket`             | 必填    | 存储桶名称                                                                    |
| `key_prefix`         | `""`    | 存储对象键的前缀路径（例如 `releases/`）                                      |
| `region`             | `auto`  | S3 存储桶所属区域                                                             |
| `access_key_id`      | 必填    | S3 访问密钥 ID                                                                |
| `secret_access_key`  | 必填    | S3 访问密钥密码                                                               |
| `force_path_style`   | `true`  | 是否强制使用路径风格 URL（MinIO 通常需设为 `true`）                           |
| `redirect_downloads` | `false` | 是否将下载请求重定向到 S3 预签名 URL（设为 `true` 可减少 RenoP 节点下行带宽） |
