---
title: Docker / OCI Registry v2 API
order: 6
category: API 参考
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

镜像页面提供包级 Markdown README。L3/L4 镜像成员或管理员可通过
`PUT /api/docker/repositories/{repo}/images?image={name}` 更新。JSON `description` 的上限为 512 KiB，并通过
共用的元素与 URL 白名单渲染。

## 版本检查

- **路径**：`GET /v2/` 或 `HEAD /v2/`
- **响应**：
    - `200 OK`，并带有 `Docker-Distribution-API-Version: registry/2.0`；
    - 需要认证时返回 `401 Unauthorized`，并带有
      `Www-Authenticate: Bearer realm="http://.../v2/token",service="renop"`。

---

## Bearer Token 认证

- **路径**：`GET /v2/token` 或 `GET /v2/auth`
- **用途**：将 Basic Auth 凭据换取短期 Docker Token。API Token 拉取需要 `repository:read`，推送需要
  `repository:publish`，删除需要 `repository:delete`；每个动作还会独立检查镜像可见性与 L0-L4 权限。

---

## 目录与标签

### 镜像列表

- **路径**：`GET /v2/_catalog`
- **JSON**：`{"repositories": ["my-org/my-app"]}`

### 标签列表

- **路径**：`GET /v2/:name/tags/list`
- **JSON**：`{"name": "my-org/my-app", "tags": ["latest", "1.0.0"]}`

---

## Manifest 操作

- **获取**：`GET /v2/:name/manifests/:reference`
- **发布**：`PUT /v2/:name/manifests/:reference`（要求镜像已创建且权限不低于 L1）
- **删除**：`DELETE /v2/:name/manifests/:reference`

Manifest JSON 上限为 4 MiB。本地上传、镜像源响应以及已持久化的 Disk/S3 对象使用同一限制；超限内容会在解析或
缓存前被拒绝。
持久化或返回前，系统还会验证声明的 SHA-256 摘要与 JSON 原始字节完全一致。

---

## Blob 操作

- **检查**：`HEAD /v2/:name/blobs/:digest`
- **下载**：`GET /v2/:name/blobs/:digest`
- **开始上传**：`POST /v2/:name/blobs/uploads/`（支持 `?mount=<digest>&from=<other_repo>`）
- **追加分块**：`PATCH /v2/:name/blobs/uploads/:uuid`
- **完成上传**：`PUT /v2/:name/blobs/uploads/:uuid?digest=sha256:...`

## 资源锁定

管理员和仓库版主通过 `PUT /api/docker/repositories/{repo}/locks?image={name}` 设置手动锁定，通过 `DELETE /api/docker/repositories/{repo}/locks?image={name}` 解除。两种操作都需要有效的浏览器会话 Cookie，API Token 不能管理锁定。请求使用已存在的不可变清单摘要；`version` 为空时锁定整个镜像。

```json
{"version":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","mode":"read","reason":"trojan"}
```

类型为 `write` 和 `read`。公开原因为 `hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、`squatting` 和 `quality`。读取锁定也会禁止写入：只有管理员、仓库版主、所有者和协作者（包括 L0）可以查看元数据，任何人都不能从该镜像下载或挂载层与配置内容。目录、搜索、标签分页、个人主页和团队资源遵守相同的可见性规则。

摘要锁定覆盖所有标签别名，以及多架构索引引用的子清单和 blob。记录的引用在重启后保留，直到解除来源锁定。检查响应通过 `locks`、`inherited`、`moderator`、`member` 和 `version_locked` 字段显示公开状态，不公开操作人员身份。解除手动锁定不会移除系统锁定或继承的限制。每次锁定最多处理 8,192 个摘要和 64 MiB 清单元数据；无效或超限的引用图返回 `400`，保留原有锁定。

锁定后的修改返回 `423` 和 `X-Renop-Error-Code: resource_locked`，包括标签改指向、发布审核通过、镜像锁定后的团队修改，以及存在版本锁定时的整镜像删除或废弃。删除或替换共享 blob 时，还会检查同仓库其他镜像的锁定。已冻结的镜像内容不会被刷新或替换。
