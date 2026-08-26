---
title: Docker / OCI Registry v2 API
order: 6
category: API 接口
description: OCI Distribution v2 与 Docker Registry v2 接口
---

# Docker / OCI Registry v2 API

RenoP 实现 OCI Distribution Spec v2 与 Docker Registry v2 规范。

容器镜像是显式资源。请求推送凭据前，必须通过 `POST /api/docker/repositories/:repo/images` 或仓库页面创建
镜像。Blob 与 Manifest 接口不会隐式创建镜像。私有镜像不出现在未授权目录中，读取 Manifest 与关联 Blob
要求 L0-L4 成员权限或管理员权限。

规范化名称已在本地或适用的已启用上游镜像中存在时，创建返回 `409 Conflict`。无法确定上游结果时不会占用
名称，并返回 `503 Service Unavailable`。

管理 API 返回可读正文与 `X-Renop-Error-Code`，前端根据稳定错误码本地化，不显示原始服务端文本。OCI
Distribution 接口继续使用规范要求的 `errors` 结构。

## 1. 版本检查

- **路径**：`GET /v2/` 或 `HEAD /v2/`
- **响应**：
    - `200 OK`，并带有 `Docker-Distribution-API-Version: registry/2.0`；
    - 需要认证时返回 `401 Unauthorized`，并带有
      `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"`。

---

## 2. Bearer Token 认证

- **路径**：`GET /v2/token` 或 `GET /v2/auth`
- **用途**：将 Basic Auth 凭据换取短期 Docker Token。API Token 拉取需要 `repository:read`，推送需要
  `repository:publish`，删除需要 `repository:delete`；每个动作还会独立检查镜像可见性与 L0-L4 权限。

---

## 3. 目录与标签

### 镜像列表

- **路径**：`GET /v2/_catalog`
- **JSON**：`{"repositories": ["my-org/my-app"]}`

### 标签列表

- **路径**：`GET /v2/:name/tags/list`
- **JSON**：`{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## 4. Manifest 操作

- **获取**：`GET /v2/:name/manifests/:reference`
- **发布**：`PUT /v2/:name/manifests/:reference`（要求镜像已创建且权限不低于 L1）
- **删除**：`DELETE /v2/:name/manifests/:reference`

---

## 5. Blob 操作

- **检查**：`HEAD /v2/:name/blobs/:digest`
- **下载**：`GET /v2/:name/blobs/:digest`
- **开始上传**：`POST /v2/:name/blobs/uploads/`（支持 `?mount=<digest>&from=<other_repo>`）
- **追加分块**：`PATCH /v2/:name/blobs/uploads/:uuid`
- **完成上传**：`PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`
