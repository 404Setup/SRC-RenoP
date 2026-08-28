---
title: 存储与上传 API
order: 10
category: API 接口
description: 存储库直接操作与有界可恢复分块上传
---

# 存储与上传 API

直接存储接口用于 Maven 与 `files` 存储库；npm、Cargo 和 Docker 使用各自原生协议。所有修改操作都会同时检查
API Token 权限、存储库权限、仓库引擎及 Maven 域策略。

## 存储库直接操作

标准路径为 `/{repo}/{path...}`。读取支持 HTTP 条件请求与字节范围。`HIDDEN` 不参与列表发现，但精确路径
仍可读取；`PRIVATE` 要求授权。

### 下载

- **请求**：`GET /{repo}/{path}` 或 `HEAD /{repo}/{path}`
- 本地缺失文件可通过已启用镜像解析，并按照配置的缓存策略写入本地。

### 上传

- **请求**：`PUT /{repo}/{path}`
- **认证**：密码，或带有 `repository:publish` 的 API Token，并要求当前账号具有写入/域权限。
- Maven 仅接受已验证域下的有效坐标与元数据。`files` 接受清理后的任意路径并支持覆盖。

### 删除

- **请求**：`DELETE /{repo}/{path}`
- **认证**：带有 `repository:delete` 的 API Token 或其他允许的凭据，并要求当前删除权限。

## 可恢复分块上传

元数据使用 protobuf，分块使用原始二进制。服务端控制最终路径，限制分块大小和会话数量，并清理废弃的
临时文件。

### 初始化

- **路径**：`POST /api/upload/chunked/`
- **Content-Type**：`application/x-protobuf`，正文为 `ChunkedUploadInitRequest`。
- `purpose` 为 `storage` 或 `updater`；storage 的 `path` 以存储库名称开头。

```json
{
  "purpose": "storage",
  "filename": "app-1.0.0.jar",
  "size": 524288000,
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "generate_checksums": true,
  "chunk_size": 4194304,
  "gpg_signature_expected": false
}
```

### 上传分块

- **路径**：`PUT /api/upload/chunked/{upload_id}/{index}`
- **Content-Type**：`application/octet-stream`。
- 分块可并发上传；重试已接收的 index 是幂等操作，长度不符的分块会被拒绝。

### 完成或中止

- **完成**：`POST /api/upload/chunked/{upload_id}/complete`
- **中止**：`DELETE /api/upload/chunked/{upload_id}`
- 完成操作只允许一个调用成功，会重新检查全部分块与权限，并通过存储库门控提交。

```json
{
  "status": "created",
  "message": "",
  "path": "releases/com/example/app/1.0.0/app-1.0.0.jar",
  "release_id": ""
}
```

Maven 强制 GPG 时，隔离阶段可返回带 `release_id` 的 `202 Accepted`。`purpose=updater` 成功时返回
`ready_to_restart`，而不是存储库路径。
