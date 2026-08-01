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
| `used_memory` / `total_memory`                         | 物理内存使用与总量（字节）                 |
| `vss_memory`                                           | 虚拟内存大小（字节）                       |
| `renop_used_disk`                                      | RenoP 存储占用                             |
| `disk_used` / `disk_total`                             | 磁盘使用与总量                             |
| `used_threads` / `available_threads` / `total_threads` | Goroutine 及并发限制相关线程数             |
| `architecture` / `os`                                  | GOARCH / GOOS                              |
| `logical_cores` / `physical_cores`                     | 逻辑与物理 CPU 核心数                      |
| `failures_count`                                       | 运行时失败计数                             |
| `update_state`                                         | 更新器状态 — 见 [updater.md](./updater.md) |
| `debug_mode`                                           | 进程启动时是否激活了 Debug 调试模式        |

## `GET /api/status/snapshots`

历史采样。响应：protobuf `StatusSnapshotList`。

| 字段           | 含义       |
|----------------|------------|
| `timestamp`    | Unix 毫秒  |
| `used_memory`  | 内存       |
| `used_threads` | 线程数     |
| `open_files`   | 打开文件数 |

无数据时返回空列表（不是 404）。

## 调试分析 API（`/api/debug`）

需具备 **manager** 权限，且配置文件中启用 `server.debug_mode: true` 并在启动时加载生效。未开启调试模式或权限不足时返回
403。

### `GET /api/debug/memory/heap`

导出 Go 运行时堆内存分析文件（pprof 格式，包含 HTTP Header 响应与附件文件）。支持查询参数 `gc=1`（默认执行垃圾回收后再采样）。

### `GET /api/debug/memory/allocs`

导出历史内存分配 Profile 文件（pprof 格式）。

### `GET /api/debug/memory/goroutine`

导出当前 Goroutine 堆栈分析文件（pprof 格式）。

### `GET /api/debug/memory/runtime`

获取 Go 运行时内存、堆/栈细分以及 Off-heap 评估数据。响应：`application/x-protobuf`，`RuntimeMemoryBreakdown`。 包含
`process_rss`、`process_vss`、`go_retained`、`heap_inuse`、`heap_alloc`、`heap_sys`、`off_heap_runtime_estimate` 等字段。
