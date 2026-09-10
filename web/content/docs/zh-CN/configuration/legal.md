---
title: 法律文档与 Cookie 偏好
order: 15
category: 配置
description: 配置协议页面、登录注册同意和浏览器偏好
---

# 法律文档与 Cookie 偏好

## 协议页面

管理员可在法律文档设置页编辑隐私政策、用户协议和法律声明。三者共用 Markdown 编辑器与安全预览，每篇最多 512 KiB UTF-8 文本。留空将恢复占位符，请替换为实例的正式文档。

文档保存在 `config.yaml` 的 `legal` 中，保存后立即生效。旧隐私文件不再读取，请在升级前将内容复制到设置中。原外部法律声明 URL 也已停用，已有文件会保留。

公开页面为 `/privacy-policy`、`/terms-of-service` 和 `/legal-notice`，过期凭据不影响阅读。`GET /api/legal` 返回隐私政策与协议的当前版本及 `cookie_banner`；`GET /api/legal/:document` 返回有大小限制的纯文本。`GET /api/privacy-policy` 保留为别名。

`GET /api/settings/legal` 和 `PUT /api/settings/legal` 需要设置管理权限，使用 JSON 字段 `privacy_policy`、`terms_of_service`、`legal_notice` 和 `cookie_banner`。

## 登录与注册

登录和注册必须明确同意当前隐私政策与用户协议，包括密码、Passkey、第三方登录及二次验证完成阶段。包客户端的密码认证仍遵守原有协议规则。

客户端通过 `GET /api/legal` 获取版本，通过 `X-Renop-Legal-Revision` 或浏览器同意 Cookie 表示接受。未同意或版本过期时返回 HTTP 428，并附带 `X-Renop-Error-Code: legal_consent_required`。成功登录或注册会在现有审计事件中记录同意版本。

## Cookie 偏好

浮窗提供仅必要、全部接受和分类偏好。必要 Cookie 用于会话及安全；可选第三方验证需要明确同意。页脚可随时重新打开偏好，包括关闭浮窗时。

分类偏好在浏览器存储中保留一年，隐私政策或协议变化后失效。拒绝可选服务是有效选择，集成必须在获准前阻止加载。
