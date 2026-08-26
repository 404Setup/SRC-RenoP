---
title: 状态与遥测 API
order: 9
category: API 接口
description: 公开健康检查、运行时指标、历史快照与受保护诊断
---

# 状态与遥测 API

标明的响应使用 protobuf。健康检查与实例状态公开可读；内存诊断要求管理员权限，并且进程启动时已启用
`server.debug_mode`。

## 1. 健康检查与前端哈希

- **健康检查**：`GET /api/status/health`，进程正常服务时返回 `"UP"`。
- **前端哈希**：`GET /api/status/hash`，返回用于检测 UI 是否需要重新加载的嵌入资源哈希。

## 2. 当前实例状态

- **路径**：`GET /api/status/instance`
- **格式**：protobuf `InstanceStatus`。
- **内容**：版本、运行时间、RSS/VSS、磁盘、goroutine、CPU、失败次数、调试状态与更新状态。

### 解码示例

```json
{
  "version": "1.0.0",
  "uptime": 86400,
  "used_memory": 33554432,
  "vss_memory": 268435456,
  "renop_used_disk": 5242880000,
  "disk_used": 107374182400,
  "disk_total": 536870912000,
  "used_threads": 24,
  "logical_cores": 16,
  "failures_count": 0,
  "debug_mode": false
}
```

## 3. 历史快照与诊断

- **历史快照**：`GET /api/status/snapshots`，返回包含时间、内存、goroutine、打开文件数与 VSS 的
  `StatusSnapshotList`。
- **Heap profile**：`GET /api/debug/memory/heap`（`?gc=0` 跳过默认的预先 GC）。
- **Allocation profile**：`GET /api/debug/memory/allocs`。
- **Goroutine profile**：`GET /api/debug/memory/goroutine`。
- **Runtime breakdown**：`GET /api/debug/memory/runtime`（`?gc=1` 先执行 GC）。

```json
{
  "snapshots": [
    {
      "timestamp": 1787731200000,
      "used_memory": 33554432,
      "used_threads": 24,
      "open_files": 18,
      "vss_memory": 268435456
    }
  ]
}
```

二进制 pprof 可使用 `go tool pprof` 或 Speedscope 打开。进程启动时未启用调试模式，即使管理员调用诊断
接口也会返回 `403`。
