---
title: 存储架构
order: 3
category: 部署
description: 本地文件系统与存储库独立的 S3 兼容对象存储
---

# 存储架构

RenoP 支持本地 Disk 与 S3 兼容对象服务。每个存储库独立选择后端；切换后端时，存储库门控会与活跃操作进行
串行化。

## 本地文件系统

根目录由 `config.yaml` 的 `storage_path` 配置，默认值为 `storage`。

### 目录组织

- **Maven/files**：`{storage_path}/{repo}/{path}`
- **Cargo**：索引与归档数据隔离在存储库目录下
- **Docker**：Blob、Manifest 与引用相互隔离，并按镜像校验

物理名称属于内部实现。应使用协议 API，不得直接修改存储目录。

### 写入可靠性

- 上传使用有界临时文件，提交前校验大小、哈希与策略；
- 文件系统支持时，最终发布使用原子操作；
- 镜像提交、删除、迁移和 GPG 发布会与后端变更同步。

---

## S3 兼容对象存储

S3 适合托管对象存储。多节点运行还需要外部数据库，以及符合 RenoP 保证范围的协调机制；仅使用 S3 不会
将单个进程变成集群。

### 服务提供方

- **AWS S3**
- **MinIO**
- **Cloudflare R2**
- 实现所需 S3 API 的其他服务

### 配置示例 (`repositories.yaml`)

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

应先创建 Bucket。凭据需要对 `key_prefix` 下对象具有读取、写入、列表与删除权限。必须使用 TLS 与密钥管理
工具，不得将访问密钥提交到 Git。

### 下载方式

- **代理流式传输 (`redirect_downloads: false`)**：RenoP 完成授权后从 S3 流式返回数据。Bucket 可保持私有，
   不会暴露 S3 URL。
- **直接跳转 (`redirect_downloads: true`)**：RenoP 完成授权后返回指向短时预签名 URL 的 `302 Found`，降低
   RenoP 带宽占用。
