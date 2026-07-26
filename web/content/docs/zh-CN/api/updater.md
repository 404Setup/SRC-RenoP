---
title: 更新器
order: 7
category: API
---

# 更新器

前缀：`/api/updater`

`GET /status` 公开；`check` / `install` / `upload` / `restart` 需要 **manager**。

相同状态也出现在 `GET /api/status/instance` 的 `update_state` 上。

典型流程：

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

响应：`application/x-protobuf`，`UpdateState`（见 `proto/api/v1/api.proto`）。

| 字段                   | 含义                                                            |
|------------------------|-----------------------------------------------------------------|
| `status`               | `idle`、`available`、`downloading`、`ready_to_restart`、`error` |
| `latest_version`       | 最新版本字符串                                                  |
| `download_url`         | 包下载 URL                                                      |
| `progress`             | 下载中为 0–100                                                  |
| `error_message`        | `status` 为 `error` 时设置                                      |
| `size`                 | 包大小（字节）                                                  |
| `estimated_disk_space` | 估计所需空闲空间（字节）                                        |
| `release_date`         | 发布日期字符串                                                  |
| `release_notes`        | 发布说明文本                                                    |
| `commit_sha`           | 源提交                                                          |
| `is_release`           | 是否 release 通道构建                                           |

## `POST /api/updater/check`

| 查询      | 默认      | 含义                   |
|-----------|-----------|------------------------|
| `channel` | `release` | `release` 或 `nightly` |

```json
{
  "has_update": true,
  "current_version": "…",
  "latest_version": "…",
  "download_url": "…",
  "channel": "release",
  "size": 12345678,
  "estimated_disk_space": 40000000,
  "release_date": "…",
  "release_notes": "…",
  "commit_sha": "…",
  "is_release": true
}
```

检查失败 → 500，`{ "error": "…" }`。

## `POST /api/updater/install`

使用当前 `download_url` 异步下载并解压。为空则回退到 nightly 默认 URL。

| 状态 | 原因                                               |
|------|----------------------------------------------------|
| 507  | 磁盘不足                                           |
| 409  | 安装已在进行（`Installation already in progress`） |

立即成功响应：

```json
{"status": "started"}
```

轮询 `/status` 获取进度。完成状态：`ready_to_restart`。

## `POST /api/updater/upload`

离线更新：multipart zip。表单字段 `file` 或 `package`；必须为 `.zip`。

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

该单请求 multipart 路径仍是小包与非 UI 客户端的默认方式。

### 多分片离线上传 — 可选

来自 Dashboard 离线更新对话框的大 zip 可通过共享会话 API 并发分块上传 （仅 manager）。小于 **8 MiB** 的包仍走单请求
`POST /api/updater/upload`。init/complete 为 **`application/x-protobuf`**
（`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`）；分片为原始字节。

分片大小由总大小动态决定（见 [storage.md](./storage.md) 多分片节）； 使用 init 响应中的 `chunk_size` / `chunk_count`。

1. `POST /api/upload/chunked/`，`purpose=updater`、`filename`（须以 `.zip` 结尾）、`size`
2. 对各分片并行 `PUT /api/upload/chunked/:id/:index`（可重试；已接受分片可再 PUT）
3. `POST /api/upload/chunked/:id/complete` — 解压二进制并设为 `ready_to_restart`

complete 的 protobuf 字段：`status=ready_to_restart`，`message=…`。

## `POST /api/updater/restart`

用已准备的更新替换二进制并重启。

未就绪 → 400（`No update ready to install`）。

```json
{"status": "restarting"}
```

之后连接会断开，属预期行为。
