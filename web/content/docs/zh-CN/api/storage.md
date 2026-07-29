---
title: 存储
order: 8
category: API
---

# 仓库存储路径

制品路径不在 `/api` 下。布局：

```text
/{repo_name}/{maven-path}
```

默认仓库：

```text
/releases/...
/snapshots/...
/private/...
```

仓库名不得与静态路由前缀冲突，例如 `api`、`js`、`css`、`svg`、`assets`、`javadoc`。

## 方法

| 方法       | 权限 | 行为                                                |
|------------|------|-----------------------------------------------------|
| GET        | 读   | 下载；浏览器以 HTML Accept 请求时可能回落到管理 SPA |
| HEAD       | 读   | 仅返回响应头                                        |
| PUT / POST | 写   | 上传或覆盖                                          |
| DELETE     | 写   | 删除；成功时返回 `204`                              |

正文上限约为 2 GiB（`BodyLimit`）。上传为流式处理。

### 上传

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

成功时返回 `201 Created`。若已禁用 redeploy 且目标文件已存在，返回 `409 Conflict`。

可选请求头 `X-Generate-Checksums: true` 会写入 `.md5`、`.sha1`、`.sha256`、`.sha512` 旁路文件。

服务端会按配置更新制品索引、可选校验和以及 S3 同步。客户端所见为标准 Maven 仓库布局。

### 分块上传（可选）

认证要求与存储写入一致：会话 cookie、Basic 或 Bearer，且对目标仓库具备写权限。

路径前缀：`/api/upload/chunked`

浏览器 UI 对 **8 MiB** 及以上的文件使用分块上传；更小的文件使用单次 `PUT`。非浏览器客户端可对任意大小开启分块会话。服务端可能将极小负载合并为单个分片。

init 与 complete 使用 **`application/x-protobuf`**（`ChunkedUploadInitRequest`、`ChunkedUploadInitResponse`、
`ChunkedUploadCompleteResponse`，定义见 `proto/api/v1/api.proto`）。分片正文为原始二进制。

1. **`POST /api/upload/chunked/`** — 创建会话（`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`）

| 字段                 | 说明                                 |
|----------------------|--------------------------------------|
| `purpose`            | `storage`（默认）                    |
| `path`               | 目标路径 `repo/…/file`               |
| `filename`           | 可选显示名称                         |
| `size`               | 总字节数                             |
| `generate_checksums` | 是否写入校验和旁路文件               |
| `chunk_size`         | 首选分片大小（可选；由服务端规范化） |

响应字段：`upload_id`、`chunk_size`、`chunk_count`、`purpose`。后续分片上传必须使用返回的 `chunk_size` 与 `chunk_count`。

**分片大小规则**（服务端，`upload.NormalizeChunkSize`）：

| 总大小    | 分片大小             |
|-----------|----------------------|
| ≤ 256 KiB | 单个分片等于文件大小 |
| ≤ 8 MiB   | 单个分片等于文件大小 |
| ≤ 32 MiB  | 4 MiB                |
| ≤ 128 MiB | 8 MiB                |
| ≤ 512 MiB | 16 MiB               |
| ≤ 2 GiB   | 24 MiB               |
| 更大      | 32 MiB（上限）       |

客户端提供的 `chunk_size` 限制在 **256 KiB … 32 MiB**。若分片数量将超过约 2048，服务端会提高分片大小。省略 `chunk_size` 或发送
`0` 时采用上表。

2. **`PUT /api/upload/chunked/:upload_id/:index`** — 原始分片正文（从 0 起编号）；允许并行  
   成功：`204`。对已接受的 index 再次 PUT 为幂等操作。

3. **`POST /api/upload/chunked/:upload_id/complete`** — 组装、更新索引、可选校验和  
   成功：`201`，正文为 `ChunkedUploadCompleteResponse`（`status=created`，`path=…`）。

4. **`DELETE /api/upload/chunked/:upload_id`** — 中止会话并丢弃临时数据（`204`）。

未完成的会话约 **15 分钟** 后过期，临时数据会被删除。

### 下载

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC 仓库无需认证。PRIVATE 仓库需要 Basic 或 Bearer 凭证。

配置镜像后，本地缺失的对象可能按各仓库的缓存与负缓存设置从上游获取。

### 删除

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## 浏览器访问

当请求携带 `Accept: text/html` 时，缺失的仓库或部分目录会回落到管理 SPA，使 `http://host/releases/...` 一类路径可打开界面。机器客户端应使用
`Accept: */*` 或省略 `Accept`，以避免收到 HTML。

## Javadoc 预览

启用时：

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

需要相应的读权限。`raw` 形式提供 jar 内文件。大小受 `max_javadoc_size_mb` 限制。

## Maven 配置示例

```xml
<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>

<distributionManagement>
<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>
<snapshotRepository>
    <id>renop</id>
    <url>http://localhost:3000/snapshots</url>
</snapshotRepository>
</distributionManagement>
```

在 `~/.m2/settings.xml` 中为对应的 server `id` 配置用户名与密码（或上传 Token）。
