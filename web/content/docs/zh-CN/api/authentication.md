---
title: 认证 API
order: 2
category: API 接口
description: 登录、注销、当前用户查询与密码修改接口
---

# 认证 API

## 1. 用户登录

- **路径**：`POST /api/auth/login`
- **认证要求**：无

### 请求体 (JSON)

```json
{
  "username": "admin",
  "password": "your_password"
}
```

### 响应

- **状态码**：`200 OK`
- **响应头**：`Set-Cookie: renop_session=<session_id>; Path=/; HttpOnly; SameSite=Lax`
- **响应体 (JSON)**：

```json
{
  "success": true,
  "session_id": "abc123xyz...",
  "user": {
    "username": "admin",
    "role": "admin",
    "permissions": ["allview", "allupdate"]
  }
}
```

---

## 2. 用户注销

- **路径**：`POST /api/auth/logout`
- **认证要求**：需已登录

### 响应

- **状态码**：`200 OK`
- **响应体**：`{"success": true}`

---

## 3. 获取当前用户信息

- **路径**：`GET /api/auth/me`
- **认证要求**：需已登录

### 响应 (JSON)

```json
{
  "username": "admin",
  "role": "admin",
  "permissions": ["allview", "allupdate"],
  "email": "admin@example.com"
}
```

---

## 4. 修改当前用户密码

- **路径**：`POST /api/auth/change-password`
- **认证要求**：需已登录

### 请求体 (JSON)

```json
{
  "old_password": "current_password",
  "new_password": "new_secure_password"
}
```

### 响应

- **状态码**：`200 OK`
- **响应体**：`{"success": true}`

---

## 5. 查询审计日志

- **路径**：`GET /api/auth/logs`
- **认证要求**：Manager 或 Admin 权限
- **查询参数**：
    - `limit`：返回条数（默认 50，最大 200）
    - `offset`：分页偏移量

### 响应 (JSON)

```json
{
  "total": 120,
  "logs": [
    {
      "id": 1,
      "username": "admin",
      "action": "LOGIN_SUCCESS",
      "ip": "192.168.1.100",
      "timestamp": 1740000000
    }
  ]
}
```
