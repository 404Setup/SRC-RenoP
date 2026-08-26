---
title: 存储库与镜像
order: 2
category: 配置
description: 仓库引擎、可见性、上游镜像、迁移与 S3 存储
---

# 存储库与镜像

存储库定义位于 `repositories.yaml`，可由 `RENOP_REPOSITORIES` 覆盖路径。管理员可在仓库管理中修改同一套
经过校验的配置。仓库名称是不可变的小写 slug，并作为 URL 的第一个路径段。

## 配置示例

```yaml
repositories:
  releases:
    name: releases
    format: maven
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: true
    mirrors: []
  crates:
    name: crates
    format: cargo
    visibility: PUBLIC
    mirrors: []
  containers:
    name: containers
    format: docker
    visibility: PRIVATE
    allow_redeployment: false
    mirrors: []
```

## 存储库字段

| 字段 | 默认值 | 说明 |
|:-----|:-------|:-----|
| `name` | 必填 | 不可变仓库 slug 与 URL 前缀 |
| `format` | `maven` | `maven`、`maven-classic`、`files`、`cargo` 或 `docker` |
| `visibility` | `PUBLIC` | `PUBLIC`、`HIDDEN` 或 `PRIVATE` |
| `allow_redeployment` | `false` | 在支持的引擎中允许 Maven 版本重发或 files/Docker 覆盖 |
| `require_gpg_signature` | `false` | Maven 发布要求通过 OpenPGP 分离签名校验 |
| `mirrors` | `[]` | 按顺序执行的上游镜像定义 |
| `s3` | 省略 | 当前仓库独立的 S3 兼容存储 |

`maven-classic` 只改变前端布局，仍执行 Maven 发布规则。`files` 为非结构化存储，不生成校验文件、POM 或
签名校验。Maven 可迁移到 `files` 并反向迁移，存储对象保持原位；切回 Maven 时重建目录并恢复保存的策略。

### 可见性

- **PUBLIC**：允许匿名读取与发现。
- **HIDDEN**：所有用户的前端目录与个人资料成员关系均不显示；精确已知路径仍可读取，管理员仍可在仓库管理
  中查看。
- **PRIVATE**：读取、列表与写入均要求显式授权。私有 Docker 镜像还会检查镜像级 L0-L4 成员关系。

## 上游镜像

本地对象缺失时，RenoP 可按顺序从已启用镜像流式获取。成功结果可直接持久化，无需将完整正文读入内存。
Cargo 与 Docker 在适用上游名称已存在时会拒绝本地创建。

```yaml
mirrors:
  - name: "central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

| 字段 | 默认值 | 说明 |
|:-----|:-------|:-----|
| `name` | 必填 | 当前存储库内唯一的镜像名称 |
| `url` | 必填 | 上游基础 URL |
| `persist` | `true` | 将成功响应写入仓库存储后端 |
| `cache_ttl_secs` | `86400` | 正缓存有效时间 |
| `negative_cache` | `true` | 缓存引擎支持的上游未命中结果 |
| `timeout_secs` | `30` | 单次上游请求超时 |
| `proxy` | `""` | 使用全局路由、`direct` 或精确命名代理 |
| `allow_artifacts` | `[]` | 按引擎解释的允许规则 |
| `deny_artifacts` | `[]` | 按引擎解释的拒绝规则，拒绝优先 |

需要凭据时使用结构化 authorization 字段，不得将密钥嵌入 `url`。

## S3 兼容存储

每个存储库可使用 Disk 或独立 S3 兼容后端。切换存储或引擎时，存储库门控会与上传、删除、GPG 提交及镜像
写入进行串行化。

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

MinIO 通常要求 `force_path_style`。启用 `redirect_downloads` 后，RenoP 完成授权并返回短时预签名跳转；否则
由 RenoP 流式代理对象。
