---
title: 认证 API
order: 2
category: API 接口
description: 浏览器会话、个人资料、登录方式、恢复代码与会话撤销
---

# 认证 API

浏览器认证使用 HttpOnly `renop_session` Cookie。个人资料与会话列表不会返回会话密钥，请求头和 URL 也不
接受该密钥。私有安全设置接口仅接受浏览器会话，不接受密码或 API Token。

## 使用密码或邮箱登录

- **路径**：`POST /api/auth/login`
- **认证**：无需认证。
- **正文**：protobuf `LoginRequest`，下方展示其 JSON 字段名。`name` 可填写用户名或私有登录邮箱。

### 请求

```json
{
  "name": "admin",
  "secret": "your_password"
}
```

### 会话结果

成功后设置带有 `HttpOnly`、`SameSite=Lax` 的 `renop_session`；检测到 HTTPS 时同时设置 `Secure`。protobuf
`SessionDetails` 包含账号权限与路由，但 `session_token` 始终为空。

## Passkey 与 GitHub 登录

- **Passkey 开始**：`POST /api/auth/fido/login/begin`
- **Passkey 完成**：`POST /api/auth/fido/login/finish`
- **GitHub 开始**：`GET /api/auth/github/start`
- **GitHub 回调**：`GET /api/auth/github/callback`
- **GitHub 可用状态**：`GET /api/auth/github/status`

只有管理员完成 OAuth 配置后才显示 GitHub 登录。RenoP 请求读取用户与组织，保存不可变 Provider ID 和当前
Principal 快照，但不会持久化 OAuth Access Token。

## 当前账号与公开个人资料

- **当前会话**：`GET /api/auth/me`
- **私有个人资料**：`GET /api/auth/profile`
- **更新用户名或昵称**：`PUT /api/auth/profile`
- **更新密码**：`PUT /api/auth/profile/password`
- **登出**：`POST /api/auth/logout`
- **公开个人资料**：`GET /api/users/:username/profile`
- **包成员关系**：`GET /api/users/:username/memberships?format=cargo|docker|maven|npm`

可见路由使用用户名，不可变用户 ID 保持内部使用。`HIDDEN` 存储库成员关系不会返回；私有成员关系只对有权
查看者显示。

## 账号安全

账号安全接口要求当前浏览器会话，并返回 `Cache-Control: no-store`。

### 邮箱与密码登录策略

- **读取状态**：`GET /api/auth/profile/security`
- **设置邮箱**：`PUT /api/auth/profile/email`
- **启用或禁用密码登录**：`PUT /api/auth/profile/password-login`
- 只有仍保留 Passkey 或 GitHub 时才能禁用密码登录；重新启用前必须已经设置密码。

### 恢复代码

- **生成**：`POST /api/auth/profile/recovery-codes`
- **重设密码**：`POST /api/auth/recovery/password`
- 系统一次显示 12 串一次性代码，只存储 Argon2id verifier。恢复时必须提供 4 串不同且未使用的代码；代码
  原子消耗，已有会话全部撤销，并重新启用密码登录。

```json
{
  "identifier": "admin@example.com",
  "codes": ["CODE-ONE", "CODE-TWO", "CODE-THREE", "CODE-FOUR"],
  "new_password": "new_secure_password"
}
```

## 登录方式管理

- **查询 Passkey**：`GET /api/auth/profile/fido`
- **注册 Passkey**：先调用 `POST /api/auth/profile/fido/register/begin`，再调用
  `POST /api/auth/profile/fido/register/finish`
- **删除 Passkey**：`DELETE /api/auth/profile/fido/:device_id`
- **查询已关联 GitHub**：`GET /api/auth/profile/github`
- **断开 GitHub**：`DELETE /api/auth/profile/github`

最后一种可用登录方式不可删除或禁用。

## 浏览器会话

- **查询**：`GET /api/auth/profile/sessions`
- **撤销单个会话**：`DELETE /api/auth/profile/sessions/:session_id`
- **撤销其他全部会话**：`POST /api/auth/profile/sessions/revoke-others`

会话列表包含公开 ID、登录方式、时间、IP 与 User-Agent，不包含 Cookie 密钥。
