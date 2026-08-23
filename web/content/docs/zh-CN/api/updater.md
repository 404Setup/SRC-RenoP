---
title: 在线更新 API
order: 13
category: API 接口
description: 检查新版本、更新通道切换与应用更新接口
---

# 在线更新 API

## 1. 检查最新版本状态

- **路径**：`GET /api/updater/status`
- **认证要求**：Admin 权限

### 响应示例 (JSON)

```json
{
  "current_version": "1.0.0",
  "channel": "release",
  "has_update": true,
  "latest_version": "1.1.0",
  "release_notes": "修复若干已知问题并优化 Docker 分块上传性能。",
  "release_date": "2026-08-20T10:00:00Z"
}
```

---

## 2. 执行在线更新

- **路径**：`POST /api/updater/apply`
- **认证要求**：Admin 权限
- **说明**：RenoP 会从官方更新源下载适合当前平台与 CPU 微架构的最新二进制文件，校验 SHA256 哈希值，替换当前执行文件并提示重启生效。
- **响应**：`200 OK`，`{"success": true, "message": "Update applied successfully. Please restart the service."}`
