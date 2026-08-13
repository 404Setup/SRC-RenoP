---
title: 更新器
order: 7
category: API
---

# 更新器

前缀：`/api/updater`

`GET /status` 公开；`check` / `install` / `upload` / `restart` 需要 **manager** 权限。

状态也会出现在 `GET /api/status/instance` 的 `update_state` 字段中。

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

响应：`application/x-protobuf`，`UpdateState`（`proto/api/v1/api.proto`）。

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

| 查询      | 默认                     | 含义                   |
|-----------|--------------------------|------------------------|
| `channel` | 设置项 `updater.channel` | `release` 或 `nightly` |

省略或非法值时使用 `updater.channel`（默认为 `release`）。

| 通道      | `info.json`                                           |
|-----------|-------------------------------------------------------|
| `nightly` | `https://mvnc.pkg.one/update/renop/nightly/info.json` |
| `release` | `https://mvnc.pkg.one/update/renop/stable/info.json`  |

包路径：`…/{nightly\|stable}/{version}/{file}`。

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

失败时返回 500，`{ "error": "…" }`。

## `POST /api/updater/install`

按当前 `download_url` 异步下载并解压。

| 状态 | 原因           |
|------|----------------|
| 507  | 磁盘空间不足   |
| 409  | 安装已在进行中 |

```json
{"status": "started"}
```

轮询 `/status` 查看进度。完成后状态为 `ready_to_restart`。

## `POST /api/updater/upload`

离线更新：multipart zip（`file` 或 `package`），须为 `.zip`。

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

### 多分片上传（可选）

大文件可使用分块上传（需要 manager 权限）。小于 **8 MiB** 的文件可直接使用单次请求 `POST /api/updater/upload`。

init/complete 请求使用 `application/x-protobuf`（`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`），分片为原始字节。

分片大小见 [storage.md](./storage.md)。使用 init 响应中的 `chunk_size` / `chunk_count`。

1. `POST /api/upload/chunked/` — 设置 `purpose=updater`、`filename`（需为 `.zip`）、`size`
2. `PUT /api/upload/chunked/:id/:index`（可并行、可重试）
3. `POST /api/upload/chunked/:id/complete` → 返回 `ready_to_restart`

## `POST /api/updater/restart`

若已有待应用的更新二进制，则应用更新后重启进程；否则仅重启当前进程。

```json
{"status": "restarting"}
```
