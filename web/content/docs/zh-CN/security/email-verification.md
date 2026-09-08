---
title: 安全邮箱验证
order: 5
category: 安全
description: 通过邮件验证码或 GitHub 验证账号的私有邮箱
---

# 安全邮箱验证

## 修改邮箱

在**账号安全**中修改私有邮箱。启用邮件服务时，保存操作会向新地址发送验证邮件。输入八位验证码后，修改才会生效；验证成功前，原邮箱仍可使用。未启用邮件服务时，保存操作直接更新邮箱。

两种方式均遵循已配置的邮箱黑名单或白名单，邮件服务关闭时也不例外。规则匹配完整邮箱或域名，不包含子域名。同一国际化域名的 Unicode 与 Punycode 写法会匹配同一规则。已被其他账号使用，或仍由已注销账号保留的地址不能被占用。

## 验证 API

以下操作需要当前浏览器 Cookie。JSON 请求体要求 `Content-Type: application/json`，上限为 4,096 字节。

| 方法 | 路径 | 请求或响应 |
|---|---|---|
| GET | `/api/auth/profile/security` | 包含 `email` 和 `email_verification_required` |
| PUT | `/api/auth/profile/email` | `{"email":"new@example.com"}`；入队后返回 `202` 和 `{id,status,ticket}`，否则返回 `200` 和账号安全状态 |
| POST | `/api/auth/profile/email/confirm` | 邮箱和验证码；返回 `200` 和更新后的账号安全状态 |
| GET | `/api/auth/mail/:id` | 邮件状态；通过发起请求的账号或 `X-Renop-Mail-Ticket` 授权访问 |

```json
{"email":"new@example.com","code":"01234567"}
```

验证码有效期为 10 分钟，允许五次错误尝试，并绑定发起请求的浏览器会话。60 秒后可以申请新验证码，但仍受 IP 发件限速约束。新邮件成功入队后，旧验证码失效；入队失败会保留旧验证码。验证码、邮件队列和 IP 限速计数在同一事务中提交。

验证码错误、过期、已使用或对应状态已变化时，返回 `400` 和 `ACCOUNT_EMAIL_CODE_INVALID`，浏览器保持登录。地址已被占用时返回 `409` 和 `ACCOUNT_EMAIL_CONFLICT`。名单策略拒绝地址时返回 `400` 和 `mail_recipient_blocked`。已撤销的会话不能确认修改；账号凭据或安全设置变化也会使待处理验证失效。

## GitHub 验证

配置 GitHub 登录后，可选择**使用 GitHub 已验证邮箱**，通过 `GET /api/auth/github/start?intent=email` 授权重新获取邮箱。该流程请求 `user:email` 权限，并读取 [GitHub 当前用户的邮箱列表](https://docs.github.com/en/rest/users/emails#list-email-addresses-for-the-authenticated-user)。

RenoP 优先选择已验证的主要联系邮箱；没有主要地址时，选择第一个已验证的联系邮箱，排除 GitHub 的 no-reply 地址。获取的地址通过本地名单检查后保存。此方式无需配置邮件发送账号，也不会建立或替换 GitHub 登录绑定。

回调只能使用一次，有效期为 10 分钟，必须由原账号、原浏览器会话返回，且账号凭据未发生变化。第三方访问令牌仅用于本次查询，不会保留。

## 运行说明

RenoP 最多保留 2,048 条待处理邮箱修改记录。新增记录前和邮件清理任务中会删除过期记录。验证码以带密钥的哈希保存；队列中的邮件内容使用私有邮件加密密钥加密。验证码和邮件查询凭据不会进入 URL、日志或浏览器存储。

修改成功后会记录个人资料变更活动。串行邮件工作线程按照场景路由和发件限制发送 `email_changed` 通知。入队成功并不等于投递成功；验证界面会显示服务商的投递进度和失败状态。
