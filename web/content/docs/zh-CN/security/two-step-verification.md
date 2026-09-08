---
title: 二次验证
order: 4
category: 安全
description: Passkey 二次验证、验证器配置、账号恢复与私有验证 API
---

# 二次验证

## 登录行为

在账号安全面板配置二次验证。密码或第三方账号验证成功后，RenoP 会要求完成已配置的二次验证，然后才签发浏览器会话。同时启用验证器和 Passkey 二次验证时，可任选一种完成此步骤。用于二次验证的 Passkey 不能单独登录；用于初次登录的 Passkey 则可以继续搭配验证器验证码。

需要二次验证时，密码或初次 Passkey 登录返回 `409`，并带有 `X-Renop-Error-Code: MFA_REQUIRED`。GitHub 返回 `/account/login?mfa=1`，保留本地 `return_to` 目标。私有 HttpOnly `renop_mfa` Cookie 仅授权待完成验证接口，不属于浏览器会话。挑战有效期为五分钟，仅可使用一次，重启后失效。进程最多保留 4,096 个挑战，每个账号最多八个。

## 配置验证器

在“验证器应用”旁选择“设置”。扫描二维码，或手动输入显示的密钥，参数为基于时间、SHA-1、六位数字、30 秒周期。二维码由 RenoP 本地生成，配置内容不会发送至外部二维码服务。请在五分钟内输入验证器中的验证码确认；未确认的配置不会改变登录策略。

验证码遵循 [RFC 6238](https://www.rfc-editor.org/info/rfc6238/)，接受服务器时间前后各一个周期。请保持服务器与设备时钟同步。成功使用过的周期不可重用，包括配置确认时使用的验证码；必要时请等待下一个验证码。账号在五分钟窗口内累计五次验证码验证失败后，该窗口结束前不再接受验证码尝试。重新申请登录挑战不会重置此限制。

## 将 Passkey 用作二次验证

启用“将 Passkey 用作二次验证”前，需先在账号中注册 Passkey，并保留密码登录或已绑定第三方账号作为初次登录方式。RenoP 会阻止禁用最后一种初次登录方式，也会阻止在此选项启用时删除最后一个二次验证 Passkey。移除最后一个 Passkey 前，请先关闭此选项。二次验证断言要求 WebAuthn 用户验证。

## 验证 API

以下 JSON POST/PUT 请求均要求 `Content-Type: application/json`，正文限制为 4,096 字节。资料修改需使用当前浏览器 Cookie，且登录时间距今不超过五分钟；较旧会话返回 `MFA_REAUTH_REQUIRED`。修改成功会撤销其他浏览器会话。账号安全响应新增布尔字段 `totp_enabled` 和 `passkey_second_factor`，不会包含验证器密钥。

| 方法 | 路径 | JSON 正文或结果 |
|---|---|---|
| GET | `/api/auth/mfa` | `{totp,passkey,expires_at}` |
| DELETE | `/api/auth/mfa` | `204` |
| POST | `/api/auth/mfa/totp` | `{"code":"012345"}` |
| POST | `/api/auth/mfa/passkey/begin` | `{}` → `{options}` |
| POST | `/api/auth/mfa/passkey/finish` | `{credential}` → session |
| POST | `/api/auth/profile/mfa/totp/begin` | `{}` → `{id,secret,uri,qr,expires_at}` |
| POST | `/api/auth/profile/mfa/totp/confirm` | `{"id":"setup-id","code":"012345"}` → security |
| PUT | `/api/auth/profile/mfa` | `{"passkey_second_factor":true}` 或 `{"totp_enabled":false}` → security |

```json
{"totp_enabled":true,"passkey_second_factor":true}
```

请勿将验证码、二维码、配置密钥或挑战 Cookie 放入 URL、日志或浏览器存储。验证错误使用 `MFA_INVALID`，服务故障使用 `MFA_UNAVAILABLE`，单独使用二次验证 Passkey 登录返回 `MFA_PRIMARY_REQUIRED`。验证成功后返回 JSON 会话信息，并设置正常的 HttpOnly 浏览器会话 Cookie。

## 恢复与运维

使用四个未使用的离线恢复代码，会在同一事务中重置密码、移除验证器、关闭 Passkey 二次验证模式并撤销全部浏览器会话。已注册的 Passkey 仍保留绑定，可重新用于初次登录。通过邮件重置密码会保留二次验证设置。请妥善保存离线恢复代码，并在恢复账号后重新配置二次验证。

启用任意二次验证的账号，须为包管理客户端和自动化使用 API 令牌；协议认证入口会拒绝账号密码。现有的限权令牌和迁移后的上传令牌继续遵循各自权限。关闭二次验证后，若密码登录仍启用，协议入口恢复接受密码。

RenoP 在启动时于配置顶层创建私有 `mfa_encryption_key`。验证器密钥使用 AES-GCM 加密，并绑定不可变用户 ID；该加密密钥不会出现在设置响应中。备份时请同时保存配置和数据库，迁移实例时保留此密钥。丢失密钥会导致验证器无法验证，可使用离线恢复代码恢复访问。
