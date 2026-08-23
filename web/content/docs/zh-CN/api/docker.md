---
title: Docker / OCI Registry v2 API
order: 6
category: API 接口
description: OCI Distribution Spec v2 与 Docker 镜像端点规范
---

# Docker / OCI Registry v2 API

RenoP 实现了 OCI Distribution Spec v2 与 Docker Registry v2 规范。

## 1. 协议版本探测

- **路径**：`GET /v2/` 或 `HEAD /v2/`
- **响应**：
    - 成功认证或公开访问：返回 `200 OK` 并附带响应头 `Docker-Distribution-API-Version: registry/2.0`。
    - 需要认证：返回 `401 Unauthorized` 并附带响应头
      `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"`。

---

## 2. Bearer Token 认证端点

- **路径**：`GET /v2/token` 或 `GET /v2/auth`
- **说明**：Docker 客户端在此端点通过 HTTP Basic Auth 换取临时的 Docker Bearer Token。

---

## 3. 镜像仓库目录与标签列表

### 查询仓库列表

- **路径**：`GET /v2/_catalog`
- **响应 (JSON)**：`{"repositories": ["my-org/my-app", "library/base"]}`

### 查询镜像标签列表

- **路径**：`GET /v2/:name/tags/list`
- **响应 (JSON)**：`{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## 4. Manifest 清单操作

- **获取 Manifest**：`GET /v2/:name/manifests/:reference`（`:reference` 可以是 tag 或 digest）
- **上传 Manifest**：`PUT /v2/:name/manifests/:reference`
- **删除 Manifest**：`DELETE /v2/:name/manifests/:reference`

---

## 5. Blob 数据层操作

- **检查 Blob 是否存在**：`HEAD /v2/:name/blobs/:digest`
- **下载 Blob 数据**：`GET /v2/:name/blobs/:digest`
- **发起上传**：`POST /v2/:name/blobs/uploads/`（支持通过 `?mount=<digest>&from=<other_repo>` 进行跨仓库快速挂载）
- **追加块上传**：`PATCH /v2/:name/blobs/uploads/:uuid`
- **完成上传并校验哈希**：`PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
