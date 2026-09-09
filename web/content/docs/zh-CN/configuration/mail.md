---
title: 邮件发送
order: 6
category: 配置
description: 邮件服务、持久队列、配额、计费、模板及管理 API
---

# 邮件发送

在**设置 → 服务**中配置邮件。添加邮件账号、选择服务预设、填写凭证并保存。
设置实例公开 HTTPS 地址后启用邮件。配置立即应用于后续队列操作，无需重启 RenoP。

## 配置

```yaml
mail:
  enabled: false
  public_url: https://packages.example.com
  site_name: RenoP
  template_style: card
  delay: {value: 5, unit: second}
  manual_rate: {limit: 1, interval: {value: 2, unit: minute}}
  account_rate: {limit: 50, interval: {value: 1, unit: minute}}
  calibration: {value: 5, unit: minute}
  list_mode: blacklist
  use_disposable_blacklist: false
  addresses: []
  accounts:
    - id: primary
      name: Main mailbox
      enabled: true
      provider: smtp
      preset: smtp-custom
      scenes: ["*"]
      from: noreply@example.com
      from_name: RenoP
      smtp_host: smtp.example.com
      smtp_port: 587
      smtp_security: starttls
      username: noreply@example.com
      password: ""
      quota: {limit: 0, period: month}
      force_send: false
      overage: {limit: -1, period: month}
      balance_micros: null
      fetch_balance: false
      pricing:
        currency: USD
        rounding: proportional
        tiers:
          - {up_to: 0, amount_micros: 100000, batch_size: 1000}
```

仅启用一个账号时，所有场景均使用该账号。启用多个账号时，明确分配的场景优先于唯一的 `*` 默认账号。
两个已启用账号不能占用同一场景。未分配的场景没有发件账号，系统不会随意选择其他账号。
每个账号使用稳定的 `id`；修改显示名称保留计数。移除账号后，工作进程处理其未发送邮件时会取消这些邮件。
禁用邮件服务或单个账号会暂停发送。已入队邮件的有效期仍然保留。

`delay` 支持秒、分钟、小时，设为 0 则不额外等待，但仍按顺序发送。
`manual_rate` 按 IP 限制手动请求，包括测试邮件；周期支持分钟、小时、日。
`account_rate` 按邮件账号统计系统及手动发件尝试，周期还支持秒。
两种限额及其周期数值必须大于 0。默认每个 IP 每两分钟一次手动请求，每个账号每分钟 50 次发件尝试。

`list_mode` 可选 `blacklist` 或 `whitelist`。策略在入队前和发送前检查。国际化域名的 Unicode 与 Punycode 写法等价，末尾的域名点号会被忽略。

| 规则 | 匹配范围 |
|---|---|
| `person@example.com` | 该完整邮箱 |
| `@example.com` | 该精确提供商域名，不包含子域名 |
| `.com` 或 `.example.com` | 完整 DNS 后缀，包含基础域名及子域名 |

设置 `use_disposable_blacklist: true` 可在黑名单规则之外启用内置临时邮箱名单。默认关闭，白名单模式忽略此选项。当前快照合并了 [disposable/disposable-email-domains](https://github.com/disposable/disposable-email-domains) 和 [disposable-email-domains/disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains) 的 75,627 个域名，不按国家或提供者筛选。名单中的域名及其子域名都会被拦截，匹配时不访问外部服务。

该名单是随版本发布的快照，无法穷尽不断出现的新提供商，可通过自定义规则补充。构建会按照 `scripts/update-disposable-domains.mjs` 中固定的版本和校验值自动生成被 Git 忽略的 `internal/mail/data/`，已有且校验通过的数据可离线复用。来源许可证见 `THIRD_PARTY_NOTICES.md`。

## 服务商与入口

| 服务商 | 预设与凭证 | 远端能力 |
| --- | --- | --- |
| SMTP | 明文 SMTP、隐式 SSL/TLS、必须升级的 STARTTLS；用户名及密码或支持的 OAuth | 本地估算、SMTP 接收响应 |
| Cloudflare Email | 账号 ID、API 令牌；全球 REST 入口 | 首次响应中的送达、拒绝或排队结果 |
| Microsoft Graph | Outlook.com、Microsoft 365/Entra；全球、美国政府 L4/L5、中国入口 | 已发送文件夹状态 |
| Amazon SES | 区域 IPv4、双栈及可用的 FIPS 入口；访问密钥、私钥、可选临时令牌 | 发件限额、邮件洞察 |
| Twilio SendGrid | 全球和欧盟入口；API 密钥 | 额度、Email Activity 状态 |
| Google Gmail | Gmail、Workspace；委托 OAuth 凭证 | 已发送标签状态 |
| 阿里云邮件推送 | 杭州、新加坡、弗吉尼亚、法兰克福；公网与 VPC 入口；访问密钥及私钥 | 免费配额、账号余额、无法精确关联的投递统计 |
| 腾讯云邮件推送 | 中国及国际版邮件、账单入口；访问密钥及私钥 | 账号余额、收件人投递状态 |
| 飞书 / Lark 邮箱 | 飞书及 Lark 入口；用户委托 OAuth 凭证 | 收件人投递状态 |

区域支持以 [SES 入口目录](https://docs.aws.amazon.com/general/latest/gr/ses.html)及[阿里云邮件推送入口目录](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint)为准。
阿里云 VPC 入口需要同区域网络连通性；预设不包含已停用的悉尼入口。
SMTP 预设包含 Gmail、Outlook.com、Microsoft 365、QQ、网易 163/126、Cloudflare、SendGrid、SES、阿里云、腾讯云及飞书。
入口和价格均可修改。切换预设会加载对应入口及计费默认值；切换服务商不会沿用另一服务商的已保存凭证。

手动配额与计划外限额会保留。预设切换货币时会清空手动余额，不会直接将原金额解释为另一种货币。

## 凭证与权限

SMTP 的 `smtp_security` 接受 `plain`、`tls`、`starttls`；隐式 TLS 的默认端口为 465，其他方式为 587。
TLS 会验证证书和服务器名称；选择 STARTTLS 后必须完成升级，明文 SMTP 需要明确选择。
服务商要求时应使用应用专用密码。OAuth SMTP 支持 Gmail、Microsoft 刷新凭证；仅填写访问令牌时，需要管理员自行更新令牌。

Graph 委托账号使用 `client_id`、可选 `client_secret` 及 `refresh_token`，个人账号可使用租户 `common`。
应用权限使用 Entra 租户 ID 和客户端凭证；此时 `mailbox` 必须为用户 ID 或邮箱地址，委托权限可以使用 `me`。
发送需要 `Mail.Send`，查询已发送文件夹需要相应的 `Mail.Read` 权限；应用权限需要管理员同意。
参见 [Graph sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail)及[国家云部署](https://learn.microsoft.com/en-us/graph/deployments)。

具体权限范围、IAM/CAM/RAM 操作、令牌获取、服务开通要求及已核对的请求契约见[邮件 API 权限](mail-api-permissions.md)。
腾讯云 API 普通账号需要通过 `tencent_template_id` 配置已审核模板；`Simple` 自定义正文属于历史受限能力。

Gmail 需要委托发件及邮件元数据权限。飞书/Lark 需要用户访问令牌，以及邮件发送和读取权限。
RenoP 串行刷新受支持的 OAuth 令牌，并在发送前持久化轮换后的刷新令牌。
API 字段包括 `api_key`、`api_secret`、可选 `session_token`、`account_id`、`region`、`endpoint` 和可选 `billing_endpoint`。
发件地址必须获得服务商授权。账号 ID、邮箱 ID 和 API 密钥是不同字段，不能互相替代。

## 配额与计费

`quota.limit` 是本地发件额度：负数为无限，0 或未填写为自动获取，正数为手动额度。
没有可用的远端配额时，自动模式初始按无限处理。校准失败会保留已知额度，不会因此补充额度。
手动周期按 UTC 自然小时、日、周、月计算，周一为每周起点。四种周期的用量分别保留。

启用 `force_send` 后，常规配额耗尽时可使用计划外配额。
`overage.limit` 为负数时不限制计划外配额，0 表示禁止；周期同样支持 `hour`、`day`、`week`、`month`。
服务商的硬性限制仍然有效，SES 技术性发件上限不能通过付费绕过。
没有配额 API 时，应手动填写已购买套餐包含的额度。

`pricing.tiers` 的 `up_to` 是递增的累计数量边界；只有最后一档可以为 0，表示没有上限。
每档按 `batch_size` 封邮件收取 `amount_micros`。单封计费将每批数量设为 1。
`rounding: proportional` 按比例分摊批次价格；`batch` 在使用一批中的第一封邮件时收取整批价格。
价格和余额使用三字母 `currency` 货币的百万分之一，1000000 表示一个货币单位。界面显示普通金额。

预设计价核对日期为 2026-09-08：[Cloudflare](https://developers.cloudflare.com/email-service/platform/pricing/) 计划外邮件为每千封 0.35 美元；[SES](https://aws.amazon.com/ses/pricing/) 发件为每千封 0.10 美元。
[SendGrid 公布的套餐](https://sendgrid.com/content/dam/sendgrid/global/en/other/sendgrid-pricing/twi121--sendgrid-pricing-pdf-st1.pdf)提供可编辑的 Essentials 50K 和 Pro 100K 计划外计价默认值；欧盟入口需要符合条件的套餐。
[阿里云邮件推送](https://www.alibabacloud.com/help/en/direct-mail/billing-methods)为每千封 0.29 美元；[腾讯云中国版](https://cloud.tencent.com/document/product/1288/47930)为每封 0.0019 元，[国际版](https://www-sg.tencentcloud.com/document/product/1084/39335)为每封 0.00028 美元。
Graph、Gmail、飞书/Lark 的边际计费预估为 0，套餐限制仍然适用。
估算不包含订阅、税费、附件流量及可选服务费用，应按照实际合同调整模型。

`balance_micros` 留空表示未知。已知余额小于等于 0，或不足以支付下一次计划外费用时，会暂停发送。
可选 `fetch_balance` 按配置的货币校准服务商余额；无法获取余额不会自动产生零余额。
发件前预留配额及计划外费用。凭证无效、发件账号配置无效、提交前连接失败不会消耗配额。
其他发件尝试会扣除配额，包括收件人拒绝和结果不确定的提交。明确不应计费的失败只退款一次，发件限速计数仍保留。
后续投递查询发现失败时，如果更新的远端校准已包含该调整，就不会再次增加远端额度。

`calibration` 默认为五分钟，支持分钟、小时、日；留空、0 或负数禁用远端配额及余额请求。
两次校准之间由 RenoP 本地扣减。启用远端校准后，账号首次发送前会先查询。
服务商没有公开配额或余额 API 时，使用本地估算和手动配置。

## 队列与投递状态

发件、OAuth 刷新、远端校准和投递查询共用一个串行工作进程，默认在一次发件尝试结束后等待五秒。
队列及账号计数可跨重启保留。数据库租约防止多个工作进程同时提交邮件。
提交结果尚未持久化就发生中断的任务会变为 `unknown`，不会自动重发。

`accepted` 表示服务商已接收请求，`sent` 表示已发送文件夹或邮件状态查询成功。
只有明确的投递结果才能标记 `delivered`。排队、暂停、查询、失败、过期、取消和未知状态分别记录。
SES、SendGrid 查询需要相应服务功能及权限；Graph、Gmail 的已发送状态不代表收件人已经收到。
飞书/Lark 使用按收件人返回明细的 `send_status` API，区分投递成功、拒收及处理中。
阿里云公开统计响应没有邮件 ID，RenoP 会查询，但返回 `unknown`，不会将另一封邮件的结果归给当前任务。
Cloudflare 直接使用初始响应，不调用不存在的单封邮件轮询接口。

查询在发送后按队列执行，并使用有界退避。反复查询失败或任务到期会产生未知结果及全局日志。
服务商失败代码保留在管理员日志中。公开任务状态不包含收件地址、邮件正文、凭证和原始诊断信息。

## 模板与通知

`template_style` 可选 `card`、`compact`、`notice`。模板采用服务界面的中性色表面、圆角卡片、胶囊按钮及明暗配色。邮件使用收件账号保存的语言；新注册及尚未保存偏好的收件人使用发起页面的 `Accept-Language`，缺省为 `en-US`。支持前端的全部 12 种语言。邮件入队时确定语言。旧版全局 `locale` 配置会被忽略，设置页面不再提供手动选择。
各样式均包含纯文本版本、经过转义的变量，以及限定为实例公开地址的 HTTPS 操作链接。

```text
registration_verify, registration_success, password_reset, password_changed,
email_verify, email_changed, quota_changed, review_status, review_requested,
permission_changed, account_banned, account_unbanned, collaboration_invitation,
super_team_invitation, pending_reviews, unusual_login, security_changed,
account_retired, notification, test
```

密码和 Passkey 变更、权限更新、封禁解封、用户配额覆盖、审核事件、邀请和站内消息使用持久事件生成邮件。
通知会去重，首次启用邮件时不会重放旧的行为日志。
新网络登录通知比较前一次登录的 IPv4 /24 或 IPv6 /56 网络，该信号表示网络变化，不是地理位置判断。
账号相关邮件会在发送前重新检查存续状态和当前安全邮箱；注销会移除该账号的排队邮件。

## 管理 API

```http
GET /api/settings/mail
PUT /api/settings/mail
GET /api/settings/mail/presets
POST /api/settings/mail/test
GET /api/settings/mail/accounts/:id
GET /api/settings/mail/jobs?limit=20&offset=0&status=failed
GET /api/settings/mail/templates/:scene?style=card
GET /api/auth/mail/:id
```

设置接口需要管理员权限。JSON 整体替换邮件配置，请求体上限为 1 MiB。
GET 和成功的 PUT 不返回凭证值，而以 `secrets_configured` 将账号 ID 映射到已配置的凭证字段名。
同一服务商的空凭证字符串表示保留；通过 `clear_secrets: {"primary":["password"]}` 明确清除已保存的凭证。
七个只写字段为 `password`、`api_key`、`api_secret`、`session_token`、`client_secret`、`access_token`、`refresh_token`。
持久加密密钥不能通过此 API 设置或读取。

测试接口使用已保存的配置，请求 JSON 如下：

```json
{"account_id":"primary","to":"receiver@example.com"}
```

入队成功返回 HTTP 202，此时尚不能确认已经送达：

```json
{"id":"opaque-job-id","status":"queued","ticket":"private-status-capability"}
```

使用返回的 `X-Renop-Mail-Ticket` 请求头轮询账号状态接口，或使用任务所有者的登录会话。
令牌应保密且不能放入 URL。所有权或令牌无效返回 404，存储暂时不可用返回 503。
账号状态包含各周期用量、尝试次数、剩余配额估算、余额、计划外费用及最近一次校准结果。
任务列表支持 `limit` 1–50、`offset` 0–10000 和可选的精确 `status` 筛选。模板预览返回主题、HTML 和纯文本。

SMTP 只有在主机和用户名均未改变时才保留已保存的凭证。

## 存储与限制

最多配置 64 个账号、1000 条收件名单，每个账号最多 20 个计费档位。
周期最长一年，配额及档位数量最多十亿封。
纯文本与 HTML 合计、服务商单次响应均限制为 128 KiB，每个任务只有一个收件人，有效期最长 24 小时。
最多保留 2048 个活跃任务及 12048 条总记录；维护会将已完成记录缩减至约 8000 条，并删除超过七天的记录。
过期 IP 计数会删除，已移除账号的状态在 24 小时后清理。

任务正文与轮换凭证由私有配置文件中的持久 `mail.encryption_key` 加密。
应同时备份该密钥和数据库，密钥丢失或被替换后，无法解密排队邮件及账号状态。
工作进程完成的任务会移除 HTML 和纯文本；中断提交的加密正文保留至历史清理。
日志和状态 API 不公开邮件正文或收件地址。
