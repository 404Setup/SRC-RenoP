---
title: 消息中心 API
order: 7
category: API 接口
description: 账号通知、未读数量、工作流操作与管理员公告
---

# 消息中心 API

所有接口均要求认证。响应默认使用 protobuf，且不会缓存。API Token 需要 `messages:read`；管理员发送公告还
要求 `admin:notifications`，并且所属账号当前具有管理员权限。

## 查询或清理消息

- **查询**：`GET /api/messages?limit=30&cursor=...`
- **清理已完成消息**：`DELETE /api/messages`
- `limit` 范围为 1 至 100；`cursor` 使用上一页返回的不透明 `next_cursor`。
- 工作流操作仍为 `pending` 的消息不会被批量清理。

### 解码后的响应示例

```json
{
  "messages": [
    {
      "id": "00000000-0000-4000-8000-000000000001",
      "kind": "announcement",
      "severity": "info",
      "title": "Maintenance",
      "body": "Maintenance starts at 02:00 UTC.",
      "action_status": "",
      "created_at": 1787731200000,
      "read_at": 0
    }
  ],
  "unread_count": 1,
  "next_cursor": ""
}
```

## 查询未读数量

- **路径**：`GET /api/messages/unread-count`
- **解码后的响应**：`{"unread_count":3}`

## 标记已读或删除消息

### 单条消息

- **标记已读**：`POST /api/messages/:id/read`
- **删除**：`DELETE /api/messages/:id`
- 删除其他账号的消息返回 `404`；删除工作流仍未完成的消息返回 `409`。

### 全部消息

- **全部标记已读**：`POST /api/messages/read-all`
- 响应包含实际更新数量。

## 发送管理员公告

- **搜索接收者**：`GET /api/messages/admin/users?q=alice`，最多返回 8 个用户名。
- **发送**：`POST /api/messages/admin`
- 向全部账号发送时设置 `all: true`；否则提供精确 `recipients`。标题、正文、严重级别和接收者数量均受
  服务端限制。

```json
{
  "recipients": ["alice", "bob"],
  "all": false,
  "severity": "warning",
  "title": "Scheduled maintenance",
  "body": "The service will restart at 02:00 UTC."
}
```

工作流邀请与系统结果由对应服务创建。团队移除通知会说明存储库及包，或 Maven 域，但不会披露执行操作的
成员。
