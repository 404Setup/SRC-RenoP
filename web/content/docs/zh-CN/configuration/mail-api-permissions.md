---
title: 邮件 API 权限
order: 7
category: 配置
description: 邮件服务商授权、已核对的 API 契约及接入要求
---

# 邮件 API 权限

本文覆盖[邮件发送](mail.md)中的八类 API 服务商，并补充 SMTP OAuth。
接口地址和权限名称已于 2026-09-08 对照文中链接的官方资料核对。
RenoP 直接发起 HTTP 请求，无需安装服务商 SDK。

## 权限对照

为配置的发信身份授予发送权限，并按所用能力添加查询权限。
提交成功不代表投递成功。查询邮箱状态的读取权限也可能让凭据持有者访问邮件内容。

| 服务商          | 发送                                         | 状态                                               | 配额 / 余额                                               |
|-----------------|----------------------------------------------|----------------------------------------------------|-----------------------------------------------------------|
| Cloudflare      | 限定账号的 Email Sending: Edit               | 首次响应                                           | RenoP 未接入对应查询                                      |
| Microsoft Graph | `Mail.Send`                                  | 扩展属性查询需要 `Mail.Read`                       | 未接入对应查询                                            |
| Amazon SES v2   | `ses:SendEmail`                              | `ses:GetMessageInsights`                           | `ses:GetAccount`；无余额查询                              |
| SendGrid        | `mail.send`                                  | `messages.read` 和 Email Activity 历史记录附加产品 | `user.credits.read`；无现金余额查询                       |
| Gmail           | `https://www.googleapis.com/auth/gmail.send` | `https://www.googleapis.com/auth/gmail.metadata`   | 无发信额度或余额查询                                      |
| 阿里云邮件推送  | `dm:SingleSendMail`                          | `dm:SenderStatisticsDetailByParam`                 | `dm:DescAccountSummary`；可选 `bss:DescribeAcccount`      |
| 腾讯云 SES      | `ses:SendEmail`                              | `ses:GetSendEmailStatus`                           | 可选 `finance:DescribeAccountBalance`；未接入发信额度查询 |
| 飞书 / Lark     | `mail:user_mailbox.message:send`             | `mail:user_mailbox.message:readonly`               | 未接入对应查询                                            |

OAuth 账号还需要首次用户授权；自动续期需要刷新令牌。
RenoP 负责刷新已保存的凭据，邮件设置不提供交互式 OAuth 回调。
请使用注册应用的授权码流程及准确登记的回调地址取得刷新令牌，并私密保存。

## Cloudflare Email

为账号启用 Email Sending，接入发信域名并配置要求的 DNS 记录。
创建限定该账号的 **Email Sending: Edit** 令牌，填入 `api_key`，并设置所属 `account_id`。
Email Routing 权限不能代替对外发送权限。域名和 DNS 配置权限与运行时发信权限分别管理。

RenoP 在 `https://api.cloudflare.com/client/v4` 下调用 `POST /accounts/{account_id}/email/sending/send`。
请求包含结构化地址及文本/HTML，读取 `message_id`、`delivered`、`permanent_bounces`、`queued` 和 `suppressed_recipients`。
服务商排队结果保持为 `queued_provider`；RenoP 未消费 Cloudflare 独立的事件订阅。
参见[配置和令牌权限](https://developers.cloudflare.com/email-service/get-started/send-emails/)
及[发送接口定义](https://developers.cloudflare.com/api/resources/email_sending/methods/send/)。

## Microsoft Graph

在邮箱所属云环境中注册 Entra 应用。个人 Outlook.com 账号使用委托授权；
Microsoft 365 组织可使用委托权限或应用权限。
委托模式申请 `Mail.Send Mail.Read offline_access`，保存 `client_id`、适用的 `client_secret` 和 `refresh_token`，设置
`mailbox: me`。
全球云中，兼容的应用注册可使用租户 `common`；组织策略可能要求管理员批准。

应用模式需要经管理员同意的应用权限 `Mail.Send` 和 `Mail.Read`。
设置真实租户 ID、客户端 ID、客户端密钥，以及目标 `mailbox` 用户 ID 或邮箱地址。
RenoP 使用 `client_credentials`，请求所选 Graph 资源的 `/.default` 范围。
应用令牌不能访问 `/me`。请通过 Exchange 支持的应用访问控制限定可访问邮箱。

| 云环境                                 | API 基址                                       | 令牌授权服务                        |
|----------------------------------------|------------------------------------------------|-------------------------------------|
| 全球 / Outlook.com / Microsoft 365 GCC | `https://graph.microsoft.com/v1.0`             | `https://login.microsoftonline.com` |
| 美国政府 L4 / GCC High                 | `https://graph.microsoft.us/v1.0`              | `https://login.microsoftonline.us`  |
| 美国政府 L5 / DoD                      | `https://dod-graph.microsoft.us/v1.0`          | `https://login.microsoftonline.us`  |
| 中国 / 世纪互联                        | `https://microsoftgraph.chinacloudapi.cn/v1.0` | `https://login.chinacloudapi.cn`    |

发信调用 `POST /me/sendMail` 或 `POST /users/{mailbox}/sendMail`，使用配置的 `from` 地址并保存已发送副本。
状态查询按 RenoP 的跟踪扩展属性过滤“已发送邮件”；`Mail.ReadBasic` 不包含这些属性。
委托发送其他邮箱的邮件还需要 `Mail.Send.Shared`、相应的共享读取权限及 Exchange Send As/Send on Behalf 权限；直接访问该邮箱还需要
Full Access。
HTTP 202 表示已接受；找到已保存的邮件只表示 `sent`。
参见 [sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail)、[扩展属性权限](https://learn.microsoft.com/en-us/graph/api/singlevaluelegacyextendedproperty-get?view=graph-rest-1.0)、
[共享发送](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user)
及[云环境地址](https://learn.microsoft.com/en-us/graph/deployments)。

## Amazon SES

在所选区域验证发信身份；向沙箱外地址发信时，需要先取得 SES 生产访问权限。
配置 IAM 访问密钥和密钥密码；临时凭据还需要会话令牌，并由管理员在到期前更新。
SES SMTP 凭据不能代替 SES API 凭据。

在允许的身份资源上授予 `ses:SendEmail`。`ses:GetAccount` 和 `ses:GetMessageInsights` 不支持按身份资源限定，资源应为 `*`。
邮件洞察需要相应的 Virtual Deliverability Manager 能力，其费用与普通发信分开计算。
RenoP 使用 AWS SigV4 签名，服务名为 `ses`，区域来自账号配置。

`POST /v2/email/outbound-emails` 返回 `MessageId`；`GET /v2/email/account` 返回 `SendingEnabled` 和滚动 24 小时发信配额。
`GET /v2/email/insights/{message_id}/` 提供收件人事件。强制发送模式也不能绕过 SES 的硬性发信限制。
参见 [IAM 操作](https://docs.aws.amazon.com/service-authorization/latest/reference/list_sesv2.html)、
[账号查询](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetAccount.html)、
[邮件洞察](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetMessageInsights.html)
及[区域地址](https://docs.aws.amazon.com/general/latest/gr/ses.html)。

## Twilio SendGrid

验证发信地址或域名，按需创建含 `mail.send`、`messages.read`、`user.credits.read` 的受限 API 密钥。
Email Activity API 需要购买额外的历史记录产品；仅有发信密钥或发信套餐不足以查询状态。
欧盟发信需要符合要求的 Pro 或更高套餐、欧盟子用户和欧盟 IP；欧盟预设应配置该子用户的密钥。

RenoP 在所选 `/v3` 基址下调用 `POST /mail/send`、`GET /messages` 和 `GET /user/credits`。
提交响应的 `X-Message-Id` 是收件人活动 ID 的前缀；状态查询还会核对准确的收件人。
查询值使用双引号，例如 `msg_id LIKE "submission-id%"`。
Credits 表示邮件额度，不是可支取的账号余额。
参见 [API 密钥权限](https://www.twilio.com/docs/sendgrid/api-reference/api-key-permissions)、
[活动查询权限](https://www.twilio.com/docs/sendgrid/api-reference/email-activity/filter-all-messages)、
[查询语法](https://www.twilio.com/docs/sendgrid/for-developers/sending-email/getting-started-email-activity-api)、
[邮件额度](https://www.twilio.com/docs/sendgrid/api-reference/users-api/retrieve-your-credit-balance)
及[区域发信](https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send)。

## Google Gmail

在 OAuth 客户端所属 Google Cloud 项目中启用 Gmail API，并配置授权同意界面。
由邮箱用户授权 `https://www.googleapis.com/auth/gmail.send` 和 `https://www.googleapis.com/auth/gmail.metadata`。
请求 `access_type=offline`；如未签发首次刷新令牌，应重新取得用户同意。
停留在 Testing 状态的外部应用可能获得七天后到期的刷新令牌；应用发布和受限权限审核要求取决于应用类型。

保存客户端 ID、客户端密钥和刷新令牌，RenoP 在 `https://oauth2.googleapis.com/token` 交换令牌。
配置的发信地址必须是授权邮箱或其已授权的发信别名。
此邮件接入未实现服务账号 JSON 密钥或全域委托 JWT 的生成。

`https://gmail.googleapis.com/gmail/v1` 下的 `POST /users/me/messages/send` 接受 base64url MIME 并返回邮件 ID。
RenoP 通过带 `format=minimal` 的 `GET /users/me/messages/{message_id}` 查询 `SENT` 标签。
这只能确认 `sent`，不能确认投递。Gmail API 请求配额与邮箱发信限制也不同。
参见[发送权限](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send)、
[元数据读取](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/get)
及 [OAuth 配置](https://developers.google.com/identity/protocols/oauth2/web-server)。

## 阿里云邮件推送

开通邮件推送，在所选区域验证域名并配置发信地址。
使用具有 `dm:SingleSendMail`、`dm:DescAccountSummary`、`dm:SenderStatisticsDetailByParam` 的 RAM 密钥；这些操作使用资源
`*`。
可选余额校准需要 **`bss:DescribeAcccount`**，其中连续三个 `c` 是官方权限名称的原有拼写。
余额 API 操作仍名为 `QueryAccountBalance`，版本 `2017-12-14`，不属于 `dm:` 权限。

RenoP 使用签名 RPC POST 请求，邮件推送版本为 `2015-11-23`。
`SingleSendMail` 返回 `EnvId`；`DescAccountSummary` 返回免费额度及账号状态。
公开的投递统计结果没有可靠的单封邮件 ID，因此查询后返回 `unknown`，不会仅按收件人推断结果。
发件人别名不得超过 15 个字符。计费地址应按账号所属商业区域选择，不直接等同于发信区域；价格币种应与返回币种一致。

参见[发送和限制](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-singlesendmail)、
[配额权限](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-descaccountsummary)、
[投递统计](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-senderstatisticsdetailbyparam)、
[余额授权](https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-queryaccountbalance)
及[服务地址](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint)。

## 腾讯云 SES

验证发信域名及地址，授予 CAM 操作 `ses:SendEmail` 和 `ses:GetSendEmailStatus`。
官方 CAM 策略中，这两项操作均使用资源 `*`。
余额读取权限是 `finance:DescribeAccountBalance`，尽管请求签名所用的 API 服务名为 `billing`。
SecretId 填入 `api_key`，SecretKey 填入 `api_secret`；使用临时凭据时，将 Token 填入 `session_token`。

普通账号必须使用审核通过的服务商模板，将其正整数 ID 填入 `tencent_template_id`。
可创建使用以下文本变量的 HTML 模板，提交服务商审核，通过后配置其 ID：

```html
<div style="padding:24px;background:#f3f5f8;font-family:Arial,sans-serif">
  <div style="max-width:600px;margin:auto;padding:24px;background:white;border:1px solid #dfe5ef;border-radius:20px">
    <p style="color:#3158c9;font-weight:bold">RenoP</p>
    <h1>{{subject}}</h1>
    <div style="white-space:pre-wrap;overflow-wrap:anywhere">{{text}}</div>
  </div>
</div>
```

RenoP 提供主题及完整的纯文本通知，替换前会转义 HTML 字符。
变量只能放在文本内容中，不得放入属性或脚本。整个 `TemplateData` JSON 限 800 UTF-8 字节；超限通知在提交前失败，不会被截断。
此示例不保证通过服务商审核。模板模式的样式由已审核的服务商 HTML 决定。
ID 为 0 时保留 `Simple`，仅供历史上取得自定义正文特批权限的账号使用，不能为普通账号开启此权限。

`SendEmail` 和 `GetSendEmailStatus` 使用版本 `2020-10-02`，以服务名 `ses` 进行 TC3 签名。
状态同时匹配邮件 ID 和收件人，区分接受、投递、拒收及暂时延迟。
`DescribeAccountBalance` 使用版本 `2018-07-09`；RenoP 将可用余额从分转换为货币百万分之一。
中国站与国际站使用各自的账号体系及币种。
参见 [SendEmail](https://intl.cloud.tencent.com/document/product/1084/39408)、
[模板和状态字段](https://intl.cloud.tencent.com/document/product/1084/39418)、
[SES CAM 操作](https://intl.cloud.tencent.com/document/product/598/57150)、
[计费权限](https://cloud.tencent.com/document/product/555/61542)
及[余额 API](https://cloud.tencent.com/document/api/555/20253)。

## 飞书及 Lark 邮箱

为已启用邮箱的组织创建并发布自建应用。
授予 `mail:user_mailbox.message:send`、`mail:user_mailbox.message:readonly` 和 `offline_access`。
将发信用户加入应用可用范围；如安全设置提供刷新令牌开关，应启用并发布，再取得用户授权。
此处使用的发信和投递状态操作必须使用 **用户**访问令牌或刷新令牌，不能使用租户令牌。

配置应用 ID、应用密钥、刷新令牌及邮箱地址或 `me`。
飞书与 Lark 的应用注册、凭据及 API 基址分别独立：
`https://open.feishu.cn/open-apis` 和 `https://open.larksuite.com/open-apis`。
RenoP 当前通过兼容的 `POST /authen/v2/oauth/token` 接口刷新，并在发送前持久化每次轮换的刷新令牌。

`POST /mail/v1/user_mailboxes/{mailbox}/messages/send` 接受带填充的 base64url MIME，返回 `data.message_id`。
`GET /mail/v1/user_mailboxes/{mailbox}/messages/{message_id}/send_status` 返回收件人明细：
4 表示已投递；3、6 表示失败；0、1、2、5 继续查询。
只有针对收件人的投递结果才能确立已投递状态。
参见[发送邮件](https://open.feishu.cn/document/server-docs/mail-v1/user_mailbox-message/send)、
[投递状态](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status)、
[Lark 投递状态](https://open.larksuite.com/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status)
及[令牌续期](https://open.feishu.cn/document/authentication-management/access-token/refresh-user-access-token)。

## SMTP OAuth

Gmail SMTP 需要授权 `https://mail.google.com/`，仅有 Gmail API 发信权限不足以使用 SMTP。
Outlook.com/Microsoft 365 SMTP 需要 `https://outlook.office.com/SMTP.Send` 和 `offline_access`。
Microsoft 365 组织或邮箱策略要求时，还必须启用 SMTP AUTH。
将 SMTP 用户名设为邮箱地址，并配置对应的客户端和刷新凭据。

RenoP 的自动 SMTP 刷新使用委托凭据，不会以 `SMTP.SendAsApp` 获取应用令牌。
可以配置由外部管理的访问令牌，但其续期仍由管理员负责。
参见 [Google SASL OAuth](https://developers.google.com/workspace/gmail/imap/xoauth2-protocol)
及 [Microsoft SMTP OAuth](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth)。

## 验证与运维

保存账号后，在管理界面发送测试邮件，查看任务最终状态及校准结果。
使用自己控制的收件箱验证实际收信。入队的 202 响应只能证明队列接受了请求。
缺少状态权限时，已接受的发信可能最终显示 `unknown`；不要仅因状态查询失败便重发。
请查看全局日志中的服务商错误码，只补充缺少的能力权限。

官方资料核对及本地 HTTP 契约测试不能证明部署所用凭据、套餐、发信域名或区域账号可用。
此次核对未使用真实服务商凭据；真实发送、额度、余额及投递查询仍需使用部署自己的账号验证。
未知限额按照[邮件发送](mail.md)中的规则处理；校准失败时保留已知限额。
