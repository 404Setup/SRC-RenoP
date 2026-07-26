---
title: 状态
order: 5
category: API
---

# 状态与健康检查

前缀：`/api/status`

无需身份认证。

## `GET /api/status/health`

```json
"UP"
```

存活探针。

## `GET /api/status/hash`

前端资源内容哈希，JSON 字符串（用于缓存破坏）。

## `GET /api/status/instance`

响应：`application/x-protobuf`，`InstanceStatus`。

| 字段                                                   | 含义                                       |
|--------------------------------------------------------|--------------------------------------------|
| `version`                                              | 二进制版本                                 |
| `development`                                          | 开发构建标志                               |
| `uptime`                                               | 自启动以来的毫秒数                         |
| `used_memory` / `total_memory`                         | 内存，约 MiB                               |
| `renop_used_disk`                                      | RenoP 存储占用                             |
| `disk_used` / `disk_total`                             | 磁盘                                       |
| `used_threads` / `available_threads` / `total_threads` | 线程 / goroutine 相关                      |
| `architecture` / `os`                                  | GOARCH / GOOS                              |
| `logical_cores` / `physical_cores`                     | CPU                                        |
| `failures_count`                                       | 运行时失败计数                             |
| `update_state`                                         | 更新器状态 — 见 [updater.md](./updater.md) |

## `GET /api/status/snapshots`

历史采样。响应：protobuf `StatusSnapshotList`。

| 字段           | 含义       |
|----------------|------------|
| `timestamp`    | Unix 毫秒  |
| `used_memory`  | 内存       |
| `used_threads` | 线程数     |
| `open_files`   | 打开文件数 |

无数据时返回空列表（不是 404）。
