---
title: 令牌与用户 API
order: 3
category: API 接口
description: 个人访问令牌 (PAT)、上传令牌与用户账号管理接口
---

# 令牌与用户 API

## 1. 获取令牌列表

- **路径**：`GET /api/tokens`
- **认证要求**：需已登录（普通用户获取自身令牌，Manager/Admin 获取全部或指定用户令牌）

### 响应 (JSON)

```json
{
  "tokens": [
    {
      "id": "tok_123456",
      "name": "CI-Deploy-Token",
      "user": "ci_bot",
      "token_type": "upload",
      "scopes": ["canupdate:releases"],
      "created_at": 1740000000,
      "expires_at": 1771536000
    }
  ]
}
```

---

## 2. 创建新令牌

- **路径**：`POST /api/tokens`
- **认证要求**：需已登录

### 请求体 (JSON)

```json
{
  "name": "Local-Maven-Token",
  "token_type": "pat",
  "scopes": ["canview:releases", "canupdate:snapshots"],
  "expires_in_days": 90
}
```

### 响应

- **状态码**：`201 Created`
- **响应体**：返回生成的明文令牌（仅此一次返回完整明文，后续不可重新读取）：

```json
{
  "id": "tok_123456",
  "token": "renop_pat_abcdef1234567890...",
  "name": "Local-Maven-Token"
}
```

---

## 3. 吊销/删除令牌

- **路径**：`DELETE /api/tokens/:id`
- **认证要求**：令牌所有者或 Admin 权限

### 响应

- **状态码**：`204 No Content`

---

## 4. 用户账号管理 (Manager / Admin)

### 查询所有用户

- **路径**：`GET /api/auth/users`
- **认证要求**：Manager 或 Admin 权限

### 创建新用户

- **路径**：`POST /api/auth/users`
- **认证要求**：Manager 或 Admin 权限
- **请求体**：
  ```json
  {
    "username": "developer1",
    "password": "InitialPassword123!",
    "role": "user",
    "permissions": ["canview:releases", "canupdate:snapshots"]
  }
  ```

### 删除用户

- **路径**：`DELETE /api/auth/users/:username`
- **认证要求**：Admin 权限
