---
title: Docker 与 OCI 镜像库
order: 3
category: 客户端指南
description: 使用 Docker 与 Podman 登录、推送与拉取 OCI 镜像
---

# Docker 与 OCI 镜像库配置

RenoP 实现了 OCI Distribution Spec v2 与 Docker Registry v2 规范，可直接作为私有容器镜像仓库使用，支持 Docker
CLI、Podman、containerd 与 nerdctl 等客户端。

## 1. 登录镜像仓库

在终端中使用 `docker login` 或 `podman login` 进行认证：

```bash
docker login localhost:3000
# 提示输入 Username: admin
# 提示输入 Password: 填入登录密码或个人访问令牌 (PAT)
```

> **注意**：如果 RenoP 未启用 HTTPS（即直接运行在 HTTP 协议上），需要在 Docker 客户端的 `/etc/docker/daemon.json` 中配置
> `insecure-registries`：
> ```json
> {
>   "insecure-registries": ["localhost:3000", "your-renop-domain:3000"]
> }
> ```

## 2. 构建、打标签与推送镜像

```bash
# 1. 标记本地镜像
docker tag my-app:latest localhost:3000/my-org/my-app:1.0.0

# 2. 推送镜像到 RenoP
docker push localhost:3000/my-org/my-app:1.0.0
```

## 3. 拉取与运行镜像

```bash
# 从 RenoP 拉取镜像
docker pull localhost:3000/my-org/my-app:1.0.0

# 运行镜像容器
docker run -d -p 8080:8080 localhost:3000/my-org/my-app:1.0.0
```

## 4. OCI 特性支持

- **多架构清单 (Multi-Arch Manifest List)**：支持推送和拉取跨不同 CPU 架构（如 `linux/amd64`, `linux/arm64`）的同一镜像标签。
- **分块上传 (Chunked Uploads)**：对于体积较大的 Layer Blob，客户端会自动分块上传，中断后可支持断点重试。
- **跨仓库挂载 (Cross-Repo Blob Mount)**：当推送同一组织下包含相同基础镜像层的镜像时，RenoP 会自动重用已有 Blob，无需重复上传。
