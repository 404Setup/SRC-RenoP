---
title: 状态与监控 API
order: 9
category: API 接口
description: 存活检查、系统运行时指标与性能分析端点
---

# 状态与监控 API

## 1. 健康检查

- **路径**：`GET /api/status/health`
- **认证要求**：无（公开端点）
- **响应**：`200 OK`，正文为文本 `"UP"`

---

## 2. 系统运行状态

- **路径**：`GET /api/status/system`
- **认证要求**：需登录（或 Manager 权限）

### 响应示例 (JSON)

```json
{
  "version": "1.0.0",
  "go_version": "go1.28-404setup",
  "uptime_seconds": 86400,
  "memory": {
    "alloc_bytes": 33554432,
    "total_alloc_bytes": 1073741824,
    "sys_bytes": 67108864,
    "num_gc": 120
  },
  "storage": {
    "total_artifacts": 1540,
    "storage_used_bytes": 5242880000
  }
}
```

---

## 3. 性能分析端点 (`debug_mode: true`)

当在 `config.yaml` 中开启 `server.debug_mode: true` 时，RenoP 会在 `/api/debug/` 下开放标准 pprof 性能分析接口：

- `GET /api/debug/pprof/`：pprof 索引页面
- `GET /api/debug/pprof/profile`：CPU 采样分析
- `GET /api/debug/pprof/heap`：堆内存分配采样
- `GET /api/debug/pprof/goroutine`：当前协程堆栈

> **安全提示**：由于 pprof 会暴露运行时内部状态，生产环境中非排查问题时请保持 `debug_mode: false`。
