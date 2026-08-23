---
title: 存储引擎选型
order: 3
category: 运维部署
description: 本地磁盘存储与 S3 兼容对象存储的特性与选型配置
---

# 存储引擎选型

RenoP 支持本地文件系统与 S3 兼容对象存储两种存储模式，不同仓库可以按需独立配置不同的存储后端。

## 1. 本地磁盘存储

本地存储适合单节点部署或对 I/O 延迟敏感的私有化环境。

### 目录组织规范

在 `config.yaml` 中设置 `storage_path`（默认值为 `storage`）。磁盘结构按照各协议标准组织：

- **Maven**：`{storage_path}/{repo_name}/{group_path}/{artifact}/{version}/{files}`
- **Cargo**：`{storage_path}/{repo_name}/crates/{crate_name}/{version}.crate`
- **Docker**：`{storage_path}/_docker/blobs/...` 与 `{storage_path}/_docker/manifests/...`

### 写入可靠性保障

- 客户端上传过程中，数据先写入 `.tmp` 临时文件并计算校验和。
- 上传完成并校验成功后，通过操作系统原子的重命名（Rename）操作移动至目标路径，防止不完整写入。

---

## 2. S3 兼容对象存储

在分布式部署、多节点共享存储或云原生部署场景下，推荐将仓库的后端存储设置为 S3 兼容对象存储。

### 支持的对象存储服务

- **AWS S3**
- **MinIO**（私有化部署对象存储）
- **Cloudflare R2**
- **阿里云 OSS / 腾讯云 COS / 华为云 OBS**（通过 S3 兼容 API 接入）

### 在 `repositories.yaml` 中配置

```yaml
repositories:
  releases:
    name: releases
    s3:
      enabled: true
      endpoint: "https://minio.internal:9000"
      bucket: "renop-storage"
      key_prefix: "releases/"
      region: "us-east-1"
      access_key_id: "ACCESS_KEY"
      secret_access_key: "SECRET_KEY"
      force_path_style: true
      redirect_downloads: false
```

### 两种下载模式对比

1. **代理流式传输 (`redirect_downloads: false`)**：
    - 客户端向 RenoP 发起下载请求，RenoP 从 S3 流式拉取数据并通过 HTTP 连接直接转发给客户端。
    - 优点：S3 存储桶无需对公网开放，完全由 RenoP 进行鉴权与流量控制。
    - 缺点：占用 RenoP 节点的下行网络带宽。

2. **直连重定向 (`redirect_downloads: true`)**：
    - 客户端向 RenoP 发起下载请求，RenoP 校验权限后返回 302 重定向至 S3 预签名下载链接（Presigned URL）。
    - 优点：制品下载流量直接由对象存储承载，极大减轻 RenoP 服务器的网络负载。
    - 缺点：客户端环境必须能够直接访问 S3 的 endpoint 地址。
