---
title: 安全验证
order: 16
category: Security
description: 配置 CAPTCHA 服务商和网页验证范围
---

# 安全验证

## 服务商与范围

在安全验证设置页配置 CAPTCHA。支持关闭、reCAPTCHA v2 选择框、reCAPTCHA v2 隐形验证、reCAPTCHA v3、Cloudflare Turnstile、hCaptcha 和 Friendly Captcha v2，请使用对应服务商的站点密钥与私钥/API 密钥。

密码登录、注册、手动邮件、创建全局团队、创建发布域和创建包可分别开启。密码登录开关不影响 Passkey 与第三方登录。注册验证码发件启用手动邮件验证时优先使用该范围，否则使用注册验证；完成注册是另一个受保护操作。

验证面向网页交互和匿名操作。已验证的 API 令牌、协议密码凭据豁免，服务端按真实凭据判断，不依赖 User-Agent。Maven 网页上传和分片初始化会检查是否创建新的目录包；已有包和授权自动发布继续遵守原有权限。

私钥不回显。服务商与站点密钥不变时留空可保留私钥；更改其中任意一项（包括关闭）会清除旧私钥。reCAPTCHA v3 默认最低分数为 0.5。在服务器域名与服务商控制台中允许实际站点域名。Friendly Captcha 可选全球或欧盟接口。

reCAPTCHA 和 Turnstile 会校验主机名。hCaptcha 的主机名字段仅用于统计，因此通过预期站点密钥校验；Friendly Captcha 校验站点密钥，并在服务商返回来源时校验来源。

```yaml
captcha:
  provider: turnstile
  site_key: YOUR_SITE_KEY
  secret_key: YOUR_SECRET_KEY
  min_score: 0.5
  friendly_region: global
  scopes:
    password_login: true
    registration: true
    manual_mail: true
    super_team_create: false
    domain_create: false
    package_create: false
```

## 浏览器验证

仅在 Cookie 偏好允许可选验证服务后，才在独立验证框架内加载第三方代码。取消、离开页面和撤回同意都会移除框架。必须通过服务商服务端校验，错误、低分、操作不匹配或域名不匹配均拒绝。

只有服务端在操作开始前明确返回需要 CAPTCHA 时，浏览器才重试一次。网络错误、邮件提交结果不明和其他错误不会自动重放操作。

## API 约定

`GET /api/captcha` 返回公开设置并写入必要的 HttpOnly 随机值 Cookie。`POST /api/captcha/verify` 接收 JSON 字段 `scope`、`provider`、`site_key` 和 `response`；验证响应值最多 16 KiB，JSON 请求体最多 32 KiB。成功返回一次性 `proof`，有效期为 120 秒。

重试受保护操作时，在 `X-Renop-Captcha` 中提供证明并携带同一浏览器 Cookie。证明绑定范围、浏览器会话和当前服务商配置。缺少证明返回 `428`，附带 `X-Renop-Error-Code: captcha_required` 和 `X-Renop-Captcha-Scope`；无效或重复使用返回 `400`。服务商故障返回 `503`。

`GET /api/settings/captcha` 和 `PUT /api/settings/captcha` 要求设置管理权限，采用 JSON。响应不返回私钥。设置立即生效，私有配置变化会使未使用的证明失效。

[reCAPTCHA](https://developers.google.com/recaptcha/docs/verify) · [reCAPTCHA v3](https://developers.google.com/recaptcha/docs/v3) · [Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/) · [hCaptcha](https://docs.hcaptcha.com/) · [Friendly Captcha](https://developer.friendlycaptcha.com/docs/v2/getting-started/verify)
