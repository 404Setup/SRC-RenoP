---
title: 认证 API
order: 2
category: API 参考
description: 浏览器会话、个人资料、登录方式、恢复代码与会话撤销
---

# 认证 API

浏览器认证使用 HttpOnly `renop_session` Cookie。个人资料与会话列表不会返回会话密钥，请求头和 URL 也不
接受该密钥。私有安全设置接口仅接受浏览器会话，不接受密码或 API Token。

浏览器登录页面为 `/account/login`。密码、Passkey 和第三方登录完成后，会返回可选查询参数 `return_to`
指定的本站路径，不保留查询参数和片段。外部地址、认证接口及超过 1,024 个字符的值会回退到 `/`
。会话过期时进入登录页；已登录用户遇到权限不足时返回首页，保留当前会话。

[二次验证](../security/two-step-verification.md)说明验证器配置、Passkey 二次验证、待完成登录响应与恢复流程。二次验证
Passkey 不算作初次登录方式。离线恢复会移除验证器并关闭 Passkey 二次验证；邮件重置密码会保留这两项设置。

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

未绑定本地账号的 GitHub 身份需要完成[账号注册](../security/registration.md)并设置密码。OAuth 请求
`read:user read:org user:email`，回调必须携带发起授权时的浏览器 Cookie。

Microsoft、Google、GitLab、Cloudflare、Stack Exchange 和自定义 OAuth 客户端使用[第三方登录 API](../security/oauth-login.md)
，支持账号绑定、注册时必要的邮箱验证、可选资料导入，以及相同的二次验证策略。

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
- **设置邮箱**：`PUT /api/auth/profile/email`；邮件入队后的确认流程及 GitHub
  验证见[安全邮箱验证](../security/email-verification.md)。
- **启用或禁用密码登录**：`PUT /api/auth/profile/password-login`
- 只有仍保留用于主要登录的 Passkey 或第三方账号时才能禁用密码登录；重新启用前必须已经设置密码。

### 邮件验证码找回密码

独立页面 `/account/forgot-password` 验证账号已记录的安全邮箱。邮件服务关闭时，该页面及申请、确认 API 返回 `404`
；登录和恢复账号页面隐藏邮件找回入口。离线恢复页面 `/account/recovery` 仍可使用。

- **可用状态**：`GET /api/auth/password-reset/status` 返回 `{"enabled":true}` 或 `{"enabled":false}`。
- **发送验证码**：`POST /api/auth/password-reset/request` 接收 `{"email":"admin@example.com"}`，邮件持久化入队后返回 `202`
  及 `{id,status,ticket}`，此时尚未发送。
- **投递状态**：`GET /api/auth/mail/:id` 通过 `X-Renop-Mail-Ticket` 请求头接收私有凭据。页面每 3 秒查询一次，最长 10
  分钟，离开页面即停止，并显示失败或未能确认的投递状态。服务商接受邮件不代表收件人已收到。凭据不得写入 URL 或浏览器存储。
- **重置密码**：`POST /api/auth/password-reset/confirm` 接收以下 JSON。两个 POST 请求均要求
  `Content-Type: application/json`，请求体上限为 4,096 字节；密码须为 6–72 个 UTF-8 字节。

```json
{"email":"admin@example.com","code":"01234567","new_password":"new_secure_password"}
```

八位验证码有效期为 10 分钟，允许五次错误尝试。除配置的 IP 发信限流外，每个邮箱还受 60
秒申请冷却限制。新验证码成功入队后才使旧码失效；入队失败则保留旧码。邮件入队、验证码存储和限流计数在同一事务中提交。邮箱验证记录最多保留
2,048 条，入队前和定期清理时会删除过期记录。

收件策略允许的有效地址均会收到相同的邮箱控制权验证邮件，包括未注册地址，因此不能通过投递状态判断账号是否存在。重置时重新检查签发时的账号身份、邮箱、密码和安全状态版本。后来注册的账号不能使用此前签发的验证码；账号注销后旧码失效。

成功时原子消费验证码、修改密码、启用密码登录并撤销浏览器会话。API 返回 `{status:"success",username}` 并清除会话
Cookie，页面返回登录；其他凭据保持绑定，封禁仍然有效。`ACCOUNT_EMAIL_CODE_INVALID` 表示验证码无效、过期、错误次数耗尽、已使用或账号状态已变化。限流返回
`429`，队列或服务故障返回 `503`。所有响应均使用 `Cache-Control: no-store`。

### 恢复代码

独立的恢复账号页面为 `/account/recovery`，可通过登录页的 **恢复账号**
入口打开，无需启用邮件服务。恢复成功后，浏览器清除原有会话并返回登录页，自动填入用户名，同时保留有效的本地 `return_to`
目标。离开页面时会清空恢复代码和密码，这些内容不会写入浏览器历史记录。

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

## 永久注销账号

`GET /api/auth/profile/retirement` 返回当前账号的注销检查结果。此接口及
`DELETE /api/auth/profile/retirement` 均仅接受浏览器会话。注销请求使用 JSON：

```json
{"confirmation":"alice"}
```

确认内容必须与当前用户名完全一致。注销成功返回 `204 No Content`；确认内容不匹配时返回 `400`，
错误码为 `ACCOUNT_RETIREMENT_CONFIRMATION`。存在未满足的条件时返回 `409`、
`ACCOUNT_RETIREMENT_BLOCKED` 及最新检查结果。检查字段包括 `eligible`、`protected_role`、
`super_team_owner_count`、`maven_domain_owner_count`、`package_owner_count` 和 `pending_review_count`。

超级管理员和仓库版主不能注销。账号不能仍持有超级团队 T4 身份、拥有使用中的 Maven 发布域、
以 L4 身份管理未弃用的软件包，或存在待处理的审核申请。请先转让所有权、关闭发布域或将软件包永久弃用。

注销后账号和用户名永久锁定，自动退出全部管理团队，解除全部第三方登录绑定，并清空 Passkey、
活跃会话、API Token、头像、恢复代码和消息。登录将返回 `ACCOUNT_DELETED`。
绑定邮箱保留 14 天，行为日志保留 30 天；到期后由定时清理任务分批释放。已发布的软件包仍可下载。

解除的第三方身份可以立即绑定到其他有效账号；此操作不会释放已注销的用户名、缩短邮箱保留期或恢复已注销资源的所有权。

超级管理员可通过 `GET /api/tokens/:name/retention` 查看保留期限，通过
`DELETE /api/tokens/:name/retention/email` 提前释放邮箱，或通过
`DELETE /api/tokens/:name/retention/audit` 提前清空行为日志。期限及完成时间字段均为 Unix 毫秒时间戳：
`deleted_at`、`email_release_at`、`email_released_at`、`audit_purge_at` 和 `audit_purged_at`。
管理员的 `DELETE /api/tokens/:name` 同样遵循上述永久注销条件。
