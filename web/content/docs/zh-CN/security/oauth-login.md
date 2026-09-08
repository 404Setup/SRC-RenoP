---
title: 第三方登录
order: 7
category: 安全
description: 配置 Microsoft、Google、GitLab、Cloudflare、Stack Exchange 和自定义 OAuth 服务
---

# 第三方登录

## 配置服务

在管理员的服务设置中打开“第三方登录”，选择服务并添加客户端。填写凭据和完整回调 URL，然后启用并保存。每个客户端使用唯一的小写 `id`，最长 32 个字符，以字母开头，可包含字母、数字、下划线和连字符。`github` 为保留值。ID 用于识别现有绑定，保存后无法通过界面修改。最多支持 32 个服务，每个账号最多保留 32 个绑定。GitHub 继续使用独立的设置区域。

向服务提供商注册 Web 应用，并允许完整回调地址 `https://renop.example/api/auth/oauth/<provider-id>/callback`。OAuth 端点及回调必须使用 HTTPS，开发时的 HTTP 回环地址除外。保存的配置立即用于新的授权；配置变更会使正在进行的授权失效。

| 预设 | 应用设置与选项 | 邮箱处理 |
|---|---|---|
| `microsoft` | [Microsoft 标识平台](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc)；`tenant` 可为 `common`、`organizations`、`consumers` 或 Entra 租户 UUID，并须与应用支持的账号类型一致。默认授权范围为 `openid profile email`。 | [UserInfo](https://learn.microsoft.com/en-us/entra/identity-platform/userinfo) 不声明邮箱已验证，必须使用 RenoP 验证码。 |
| `google` | [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect)；默认授权范围为 `openid profile email`。 | 只有 `email_verified` 为布尔值 `true` 时，才将地址视为已验证。 |
| `gitlab` | [GitLab OpenID Connect](https://docs.gitlab.com/integration/openid_connect_provider/)；`base_url` 默认为 `https://gitlab.com`，也可指定自托管实例。默认授权范围为 `openid profile email`。 | 未返回已验证的联系邮箱时，必须使用验证码。 |
| `cloudflare` | [创建 OAuth 客户端](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/)并[完成集成](https://developers.cloudflare.com/fundamentals/oauth/integrate-with-cloudflare/)；`openid` 范围提供稳定的用户标识。私有客户端仅允许所属 Cloudflare 账号的成员使用；公开客户端需要通过 Cloudflare 的域名验证。 | 此预设不提供已验证的邮箱，必须使用 RenoP 验证码。 |
| `stackexchange` | 注册 [Stack Apps 应用](https://stackapps.com/help/api-authentication)，填写客户端凭据和 API 密钥，选择 `site`，默认为 `stackoverflow`。RenoP 使用带 PKCE 的[授权码流程](https://api.stackexchange.com/docs/authentication)，并以网络级 `account_id` 作为身份。 | 不提供联系邮箱，必须使用 RenoP 验证码。 |
| `custom` | 配置授权、令牌和用户信息 URL，以及 JSON 字段路径。使用 OpenID Connect 时，还需配置签发者和 JWKS。 | 只有邮箱字段及其验证字段均已映射，且验证值为布尔值 `true` 时才可信；否则必须使用验证码。 |

Microsoft 根据租户和应用注册设置支持个人账号与 Entra ID 账号。Cloudflare 和 Stack Exchange 无需填写虚构的邮箱映射。如果需要允许这类用户注册，请先启用[邮件发送](../configuration/mail.md)。

## 配置与凭据

服务保存在 `server.oauth_providers` 中。以下示例在替换凭据之前保持客户端禁用：

```yaml
server:
  oauth_providers:
    - id: microsoft
      type: microsoft
      name: Microsoft
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/microsoft/callback
      tenant: common
    - id: example
      type: custom
      name: Example Identity
      enabled: false
      client_id: ""
      client_secret: ""
      callback_url: https://renop.example/api/auth/oauth/example/callback
      authorize_url: https://identity.example/authorize
      token_url: https://identity.example/token
      userinfo_url: https://identity.example/userinfo
      scopes: ""
      token_auth: client_secret_post
      disable_pkce: false
      claims:
        subject: user.id
        username: user.username
        name: user.name
        email: user.email
        email_verified: user.email_verified
        avatar: user.picture
```

`name` 最长 80 个 UTF-8 字节，`client_id` 最长 512 字节，密钥和 API 密钥最长 4,096 字节。`scopes` 是空格分隔的字符串，最长 1,024 字节。端点 URL 最长 2,048 字节。内置预设提供官方端点和字段映射；其他结构应使用 `custom`。

自定义 `claims` 通过点分隔的 JSON 路径和数字数组下标选择标量值，例如 `user.id` 或 `items.0.id`，路径最长 128 个字符。`subject` 必须是服务提供商的稳定标识，不得以姓名或邮箱代替身份。数字标识保留完整精度。可省略可选的资料字段。字符串 `"true"` 不会使邮箱通过验证。

自定义 `token_auth` 可为 `client_secret_post`、`client_secret_basic` 或 `none`。默认启用 PKCE S256。只有已配置客户端密钥的自定义服务才能设置 `disable_pkce: true`。使用 OpenID Connect 时，同时填写 `issuer`、`jwks_url`，并在 `scopes` 中包含 `openid`；此时 RenoP 会要求并验证 ID 令牌。仅使用 OAuth 的自定义客户端以经认证的用户信息响应作为身份依据。

仅使用 OAuth 的自定义客户端会将身份字段路径纳入绑定的身份边界。修改该字段后需要重新关联账号。如果从提交 `26a1c1a` 开始启用了这类客户端，更新到此隔离规则后也需要重新关联。OIDC 客户端继续使用经过验证的 ID 令牌用户标识。Cloudflare 只提供 `sub`，因此不显示邮箱和头像导入操作；注册时需手动填写资料并使用 RenoP 邮箱验证码。

读取设置时会返回 `client_secret_configured` 和 `api_key_configured`，但密钥值为空。只有 ID、服务类型、客户端 ID 和令牌端点均未变化时，空值写入才会保留已保存凭据。`clear_client_secret` 和 `clear_api_key` 可明确删除凭据。更换客户端或端点后必须重新填写。移除服务会停止新的授权，但保留账号绑定供用户自行解绑。

## 注册与账号操作

尚未绑定的第三方身份登录时会进入[账号注册](./registration.md)。用户确认并设置密码之前，不会创建账号或登录会话。所有服务都遵守十分钟确认期限、第三方注册冷却期、IP 配额、收件人规则、本地用户名和昵称限制，以及头像配额。系统不会因邮箱相同而自动选择或合并现有账号。

待确认响应包含 `provider`、`provider_name`、`email_required`、`mail_enabled` 和 `avatar_available`。当 `email_required` 为 true 时，即使服务返回了未验证的建议地址，用户仍须填写并验证邮箱。向注册验证码端点同时发送 `provider` 和 `email`；最终确认仍须使用相同服务 ID，并包含收到的 `code`。重新发送验证码不会延长原确认期限，也不会重置错误次数。邮件不可用时，需要验证邮箱的服务无法完成注册。

服务返回已验证地址时，确认期间该地址固定，无需验证码。导入用户名、昵称和头像是可选操作；缺少头像或头像超限不会取消注册。名称仍须满足实例限制，重名时必须手动修改用户名。

在个人资料编辑页面，可以绑定或刷新服务、导入头像、使用最新验证的邮箱，或解除绑定。邮箱验证必须重新授权，不会改变登录绑定。没有邮箱验证字段的服务不会显示此操作。导入头像要求所选第三方身份已绑定当前账号。解绑前，服务器确保另有一种主要登录方式可用。[二次验证](./two-step-verification.md)、封禁和永久注销同样适用于第三方登录。

## API

| 方法 | 路径 | 约定 |
|---|---|---|
| GET | `/api/auth/oauth/providers` | 公开返回已配置服务的 ID 和名称 |
| GET | `/api/auth/oauth/:provider/start` | `intent=login`、`register`、`link`、`email` 或 `avatar`；可选本地路径 `return_to` |
| GET | `/api/auth/oauth/:provider/callback` | 一次性授权码和与浏览器绑定的状态 |
| GET | `/api/auth/profile/oauth` | 私有绑定状态、显示名称、授权时间和允许的操作 |
| DELETE | `/api/auth/profile/oauth/:provider` | 解绑返回 `204`；移除最后登录方式时返回 `409` 和 `oauth_last_login_method` |
| GET | `/api/settings/oauth-providers` | 管理员读取 `providers` 和 `presets`，不含密钥 |
| PUT | `/api/settings/oauth-providers` | 管理员发送 JSON `{providers:[...]}` 替换服务列表；请求体上限为 128 KiB |

个人资料操作需要当前浏览器会话。回调在 `oauth` 中返回稳定的结果标识，在 `provider` 中返回服务 ID；SPA 翻译提示后移除这些参数。失败时不会显示原始服务响应。会话登录方式为 `oauth:<provider-id>`，可附加 `+totp` 或 `+passkey`。

## GitLab Maven 授权

最近一小时内的 GitLab.com 授权可自动验证新建的 `io.gitlab.<namespace>` 域名。RenoP 接受账号本人的命名空间，以及 GitLab 群组所有者声明中列出的顶层群组。公开群组的成员资格、子群组所有权和自托管 GitLab 身份，都不能授权父级或 GitLab.com 命名空间。刷新绑定可更新所有权证明。每个身份最多保存 1,001 个命名空间。原有公开个人简介或群组描述验证仍然可用。

## 安全与运行

OAuth 回调使用十分钟 HttpOnly Cookie、服务器保存的一次性状态和 PKCE，最多保留 2,048 条外部授权状态。配置变更使待处理回调和注册失效。OIDC 会验证签名、签发者、受众、用户标识、nonce、时间范围，以及提供时的令牌哈希；支持 RS256 和 ES256 签名。服务响应限制为 1 MiB，请求有明确超时，并遵循配置的出站代理。

RenoP 将稳定用户标识与服务类型、客户端、端点和已验证签发者共同确定的授权主体绑定。变更这些身份边界不会把现有绑定转交给其他服务。登录不会保留访问令牌。受保护头像所需令牌仅可加密保存在待确认注册中，直到确认或过期；临时值由私有 `mfa_encryption_key` 保护，绝不会返回浏览器。

账号注销会原子释放全部第三方绑定。其他有效账号可以绑定被释放的身份，但原用户名仍永久保留，邮箱仍占用 14 天，行为记录仍保留 30 天。延迟回调或刷新不能为已注销账号重建绑定。
