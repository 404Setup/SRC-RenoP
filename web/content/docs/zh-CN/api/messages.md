---
title: 消息中心 API
order: 7
category: API 接口
description: 用户通知消息列表、未读统计与标记已读接口
---

# 消息中心 API

用于查询和处理当前登录用户的系统通知与工作流消息。

## 1. 获取消息列表

- **路径**：`GET /api/messages`
- **认证要求**：需已登录
- **查询参数**：
    - `limit`：单页条数（默认 30，最大 100）
    - `cursor`：分页游标（上一页返回的 `next_cursor`）

### 响应示例 (JSON)

```json
{
  "messages": [
    {
      "id": "msg_01",
      "title": "CI 构建发布成功",
      "body": "制品 com.example:lib:1.0.0 已成功部署至 releases 仓库。",
      "severity": "info",
      "read": false,
      "created_at": 1740000000
    }
  ],
  "unread_count": 1,
  "next_cursor": ""
}
```

---

## 2. 获取未读消息计数

- **路径**：`GET /api/messages/unread`
- **认证要求**：需已登录
- **响应 (JSON)**：`{"unread_count": 3}`

---

## 3. 标记消息为已读

### 标记单条消息

- **路径**：`POST /api/messages/:id/read`
- **认证要求**：需已登录
- **响应**：`200 OK`，`{"success": true}`

### 标记全部消息已读

- **路径**：`POST /api/messages/read-all`
- **认证要求**：需已登录
- **响应**：`200 OK`，`{"success": true}`

---

## 4. 发送系统通知 (Admin 权限)

- **路径**：`POST /api/messages/broadcast`
- **认证要求**：Admin 权限
- **请求体 (JSON)**：
  ```json
  {
    "recipients": [],
    "all": true,
    "severity": "warning",
    "title": "系统维护通知",
    "body": "服务器将于本周日凌晨进行配置升级维护。"
  }
  ```
