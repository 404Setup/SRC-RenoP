---
title: 存储
order: 8
category: API
---

# 仓库存储路径

制品不在 `/api` 下。布局：

```text
/{repo_name}/{maven-path}
```

默认仓库：

```text
/releases/...
/snapshots/...
/private/...
```

仓库名不得与静态路由冲突：`api`、`js`、`css`、`svg`、`assets`、`javadocs` 等。

## 方法

| 方法       | 权限 | 行为                                     |
|------------|------|------------------------------------------|
| GET        | 读   | 下载；浏览器 HTML 请求可能回落到管理 SPA |
| HEAD       | 读   | 仅响应头                                 |
| PUT / POST | 写   | 上传 / 覆盖                              |
| DELETE     | 写   | 删除；成功 204                           |

正文上限约 2 GiB（`BodyLimit`）；上传为流式。

### 上传

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

典型成功：`201 Created`。若禁用 redeploy 且文件已存在，服务器拒绝覆盖（将任何 非 2xx 视为失败）。

可选头：`X-Generate-Checksums: true` 写入 `.md5` / `.sha1` / `.sha256` / `.sha512` 旁路文件。

服务器维护索引、可选校验和与 S3 同步。Maven 客户端看到标准仓库布局。

### 多分片（分块）上传 — 可选

上面的单请求 `PUT` 不变。对大文件，Web UI 可能使用并发分块上传 （分片可重试）。机器客户端可使用相同 API。

**何时用多分片：** 浏览器 UI 对 **8 MiB** 以下文件不分块（单次 `PUT` 更快）。机器 客户端仍可对任意大小开启分块会话；服务器会将极小负载合并为单分片。

前缀：`/api/upload/chunked`（会话 cookie / Basic / Bearer；需要对目标仓库的写权限）。

init 与 complete 使用 **`application/x-protobuf`**（`ChunkedUploadInitRequest` /
`ChunkedUploadInitResponse` / `ChunkedUploadCompleteResponse`，见 `proto/api/v1/api.proto`）。分片正文为原始二进制。

1. **`POST /api/upload/chunked/`** — 开始会话（`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`）

逻辑字段（snake_case）：

| 字段                 | 含义                                 |
|----------------------|--------------------------------------|
| `purpose`            | `storage`（默认）                    |
| `path`               | 目标 `repo/…/file`                   |
| `filename`           | 可选显示名                           |
| `size`               | 总字节数                             |
| `generate_checksums` | 写入校验和旁路文件                   |
| `chunk_size`         | 首选分片大小（可选；服务器会规范化） |

响应字段：`upload_id`、`chunk_size`、`chunk_count`、`purpose`。后续 `PUT` 务必使用返回的
`chunk_size` / `chunk_count`。

**分片大小规则**（服务器，`upload.NormalizeChunkSize`）：

| 总大小    | 典型分片大小      |
|-----------|-------------------|
| ≤ 256 KiB | 单分片 = 文件大小 |
| ≤ 8 MiB   | 单分片 = 文件大小 |
| ≤ 32 MiB  | 4 MiB             |
| ≤ 128 MiB | 8 MiB             |
| ≤ 512 MiB | 16 MiB            |
| ≤ 2 GiB   | 24 MiB            |
| 更大      | 32 MiB（最大）    |

客户端 `chunk_size` 限制在 **256 KiB … 32 MiB**。若会产生超过约 2048 个分片，服务器会提高 分片大小。省略 `chunk_size`（或发
`0`）以接受上表。

2. **`PUT /api/upload/chunked/:upload_id/:index`** — 原始分片正文（从 0 起），可并行  
   成功：`204`。对已接受 index 再 PUT 幂等（可安全重试）。

3. **`POST /api/upload/chunked/:upload_id/complete`** — 组装、索引、可选校验和  
   成功：`201` + `ChunkedUploadCompleteResponse`（`status=created`，`path=…`）。

4. **`DELETE /api/upload/chunked/:upload_id`** — 中止并丢弃临时数据（`204`）。

未完成会话约 **15 分钟** 过期（临时文件删除）。客户端应对失败分片做退避重试。

### 下载

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC 无需认证。PRIVATE 使用 Basic / Bearer。

配置镜像时，本地缺失的文件可能从上游拉取（按仓库配置缓存 / 负缓存）。

### 删除

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## 浏览器访问

带 `Accept: text/html` 时，缺失的仓库或部分目录会落到管理 SPA，使
`http://host/releases/...` 可打开 UI。机器客户端应使用 `Accept: */*` 或省略 Accept 以避免 HTML。

## Javadoc 预览

启用时：

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

需要匹配的读权限。`raw` 提供 jar 内文件。大小受 `max_javadoc_size_mb` 限制。

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

在 `~/.m2/settings.xml` 中为该 server id 设置用户名 + 密码（或上传令牌）。
