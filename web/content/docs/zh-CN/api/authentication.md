---
title: 认证 API
order: 2
category: API 参考
description: 浏览器会话、个人资料、登录方式、恢复代码与会话撤销
---

# 认证 API

浏览器端认证使用具备 `HttpOnly` 属性的 `renop_session` Cookie。为保障安全性，用户信息与会话列表中均不包含会话私钥，接口也不支持通过请求头或 URL 参数传递该密钥。账号核心安全设置仅限有效浏览器会话访问。

Web 登录入口为 `/account/login`，支持通过 `return_to` 参数指定登录成功后跳转的本站站内路径（外部链接或无效路径将默认重定向至首页）。会话失效时自动引导至登录页，权限不足时返回首页并保留当前登录态。

关于验证器配置、二次验证、等待验证响应及离线恢复机制，请参阅[二次验证](../security/two-step-verification.md)。离线恢复时将重置双重验证；通过邮件找回密码则继续保留原有二步验证设置。

## 使用密码或邮箱登录

- **路径**：`POST /api/auth/login`
- **认证**：无需认证。
- **正文**：JSON / protobuf `LoginRequest`，下方展示其 JSON 字段名。`name` 可填写用户名或私有登录邮箱。

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
- **GitHub 开始**：`GET /api/auth/oauth/github/start` (`GET /api/auth/github/start`)
- **GitHub 回调**：`GET /api/auth/github/callback`
- **GitHub 可用状态**：`GET /api/auth/github/status`

GitHub 登录需由管理员配置 OAuth 参数后启用。RenoP 仅读取公开用户与组织基础资料，保存不可变 Provider ID 与账号快照，不持久化 OAuth Access Token。

首次使用未绑定的 GitHub 账号登录时，需先完成[账号注册](../security/registration.md)并设置密码。授权请求范围为 `read:user read:org user:email`，回调过程严格校验安全会话 Cookie。

Microsoft、Google、GitLab 等其他身份提供商请参阅[第三方登录 API](../security/oauth-login.md)，支持账号绑定、邮箱所有权校验与统一的二次验证流程。

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

独立找回密码页面为 `/account/forgot-password`。当系统邮件服务未启用时，该功能相关端点返回 `404`，
同时界面自动隐藏邮件找回入口。离线恢复仍可通过 `/account/recovery` 进行。

- **可用状态**：`GET /api/auth/password-reset/status` 返回当前是否允许邮件找回密码。
- **发送验证码**：`POST /api/auth/password-reset/request` 传入 `{"email":"admin@example.com"}`，
  验证码生成并加入发送队列后返回 `202` 及发件任务标识。
- **投递状态**：`GET /api/auth/mail/:id` 通过 Header 凭据 `X-Renop-Mail-Ticket` 查询邮件外发状态。
- **重置密码**：`POST /api/auth/password-reset/confirm` 接收重置凭据并应用新密码。
  密码长度需为 6–72 字符。

```json
{"email":"admin@example.com","code":"01234567","new_password":"new_secure_password"}
```

验证码为 8 位数字，有效期 10 分钟，最多允许尝试 5 次。
为防止滥用，同邮箱具备申请冷却限制与 IP 发信速率保护。
新验证码入队后旧码失效，保证重置凭证的有效性与时效性。

为防范账号探测，针对有效格式的邮箱均采用一致的应答策略。
验证成功后，新密码立即生效，同时撤销所有活跃的浏览器会话以确保安全。
验证码无效、过期或达到尝试上限时返回 `ACCOUNT_EMAIL_CODE_INVALID`。
限流时返回 `429`，服务异常时返回 `503`。

### 恢复代码

离线恢复页面为 `/account/recovery`，无需依赖外部邮件服务。
使用恢复代码成功重置密码后，浏览器将重定向至登录页并自动带入用户名。

- **生成**：`POST /api/auth/profile/recovery-codes`
- **重设密码**：`POST /api/auth/recovery/password`
- 系统一次性签发 12 组恢复代码（服务端仅保存 Argon2id 哈希）。
  重置时需提供其中 4 组未使用的代码；验证通过后代码立即失效并撤销当前所有会话。

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
- **断开 GitHub**：`DELETE /api/auth/profile/oauth/github` (`DELETE /api/auth/profile/github`)

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

确认内容必须与当前用户名完全一致。注销成功返回 `204 No Content`；
确认信息不匹配返回 `400`（`ACCOUNT_RETIREMENT_CONFIRMATION`）；
若存在未满足的注销前置条件，返回 `409`（`ACCOUNT_RETIREMENT_BLOCKED`）及详细检查项。
检查字段包含 `eligible`、`protected_role`、`super_team_owner_count`、
`maven_domain_owner_count`、`package_owner_count` 与 `pending_review_count`。

超级管理员和仓库版主需先卸任管理职责方可注销。
账号不得持有处于活跃状态的超级团队所有者（T4）身份、
Maven 发布域所有权、未弃用软件包的管理权限或待处理的审核工单。

注销后账号与用户名将被永久锁定，自动退出所有团队、解绑全部第三方登录，
并清理关联的 Token、会话凭据及消息通知。
已发布的开源软件包仍保持可用。
绑定邮箱保留 14 天，行为日志保留 30 天后自动释放。

管理员可通过 `GET /api/tokens/:name/retention` 查看保留期限，通过
`DELETE /api/tokens/:name/retention/email` 提前释放邮箱，或通过
`DELETE /api/tokens/:name/retention/audit` 提前清空日志。
时间字段均采用毫秒级时间戳。
管理员执行 `DELETE /api/tokens/:name` 时同样遵循注销检查规则。

[法律文档与 Cookie 偏好](../configuration/legal.md)

[安全验证](../security/captcha.md)
