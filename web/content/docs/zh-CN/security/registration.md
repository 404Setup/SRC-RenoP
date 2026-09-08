---
title: 账号注册
order: 6
category: 安全
description: 启用注册、邮箱确认和 GitHub 账号创建
---

# 账号注册

## 注册设置

注册默认关闭。管理员可在服务设置中启用。关闭后，`/account/register` 和注册操作返回 `404`，公开状态接口仍然可用。以下为顶层配置的默认值：

```yaml
registration:
  enabled: false
  ip_limit: 1
  ip_interval: {value: 3, unit: week}
  provider_cooldown: {value: 12, unit: hour}
```

仅成功注册扣减 IP 配额。默认每个 IP 三周可注册一个账号，周期从首次成功注册开始。等价的 IPv4 和 IPv6 写法共用限额。注销账号不会恢复此额度；限额在重启后保留。账号数量必须为 1–10,000，周期必须为正数，单位为 `minute`、`hour`、`day`、`week` 或 `month`，最长 365 天；一个月按 30 天计算。

## 创建账号

从登录页打开注册页面。用户名为 4–18 个 ASCII 字母、数字或下划线，并以小写保存。昵称可选，最多 36 个 Unicode 字符。密码必填，长度为 6–72 个 UTF-8 字节。

启用邮件服务时，必须填写邮箱及收到的八位验证码。验证码十分钟内有效，最多允许输错五次；确认时需使用同一浏览器和 IP 地址。邮件使用现有串行队列、收件人策略、配额和限速，页面显示发送状态。未启用邮件服务时，邮箱可选。成功后使用新凭据登录；启用邮件服务时还会排队发送注册成功通知。

## 通过 GitHub 注册

未绑定本地账号的 GitHub 登录会进入确认页面。RenoP 请求 `read:user read:org user:email`，选择已验证的真实联系邮箱，排除 no-reply 地址，无需再输入邮箱验证码。授权通过十分钟有效的 HttpOnly Cookie 绑定发起操作的浏览器，每个回调状态仅可使用一次。

必须在十分钟内确认并设置密码，确认前不会创建可用账号。超时后清除待确认的个人资料，并从到期时起限制同一 GitHub 身份再次发起注册，默认冷却时间十二小时。可选择导入用户名、昵称和头像；本地名称限制仍适用，用户名不可用时须手动填写。缺少可选资料或头像超出大小、配额限制，不影响注册。

## 注册 API

公开 JSON 请求要求 `Content-Type: application/json`，请求体最多 4,096 字节。确认还需浏览器的 HttpOnly 注册 Cookie；不要在浏览器存储中保存密码、验证码或邮件查询凭据。

- `GET /api/auth/registration/status`: 读取可用状态及邮箱要求.
- `GET /api/auth/registration/pending`: 读取当前浏览器的待确认注册.
- `POST /api/auth/registration/code`: 排队发送验证码；返回 `202` 和私有邮件回执.
- `POST /api/auth/registration`: 确认注册.
- `GET /api/settings/registration`, `PUT /api/settings/registration`: 读取或更新注册设置（管理员）.

```json
{
  "username": "new_user",
  "nickname": "New User",
  "email": "user@example.com",
  "password": "a unique long password",
  "code": "12345678"
}
```

确认 GitHub 注册时使用 `provider: "github"`，保留已验证邮箱并省略 `code`；设置 `import_avatar` 为 `true` 可请求导入头像。成功返回 `201`，包含 `username` 和 `avatar_imported`，不会创建登录会话。冲突返回 `409`，确认无效或到期返回 `400`，IP 额度耗尽或第三方冷却期返回 `429`。账号、邮箱、第三方绑定、IP 计数和确认凭据消费在同一事务内提交。
