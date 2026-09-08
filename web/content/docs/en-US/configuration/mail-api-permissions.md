---
title: Email API Permissions
order: 7
category: Configuration
description: Provider authorization, verified API contracts, and email integration requirements
---

# Email API Permissions

This guide covers the eight API providers in [Email Delivery](mail.md), plus SMTP OAuth.
Endpoints and permission names were checked against the linked official references on 2026-09-08.
RenoP makes HTTP requests directly; installing a provider SDK is unnecessary.

## Permission Matrix

Grant sending permission to the configured identity. Add the query permissions for the capabilities you use.
A successful submission does not establish delivery. Read permissions for mailbox status can expose mailbox content to the credential holder.

| Provider | Sending | Status | Quota / balance |
| --- | --- | --- | --- |
| Cloudflare | Account-scoped Email Sending: Edit | Initial response | No supported lookup in RenoP |
| Microsoft Graph | `Mail.Send` | `Mail.Read` for extended-property queries | No supported lookup |
| Amazon SES v2 | `ses:SendEmail` | `ses:GetMessageInsights` | `ses:GetAccount`; no balance lookup |
| SendGrid | `mail.send` | `messages.read` and Email Activity history add-on | `user.credits.read`; no cash-balance lookup |
| Gmail | `https://www.googleapis.com/auth/gmail.send` | `https://www.googleapis.com/auth/gmail.metadata` | No sending-allowance or balance lookup |
| Alibaba Direct Mail | `dm:SingleSendMail` | `dm:SenderStatisticsDetailByParam` | `dm:DescAccountSummary`; optional `bss:DescribeAcccount` |
| Tencent SES | `ses:SendEmail` | `ses:GetSendEmailStatus` | Optional `finance:DescribeAccountBalance`; no supported sending-allowance lookup |
| Feishu / Lark | `mail:user_mailbox.message:send` | `mail:user_mailbox.message:readonly` | No supported lookup |

OAuth accounts also need an initial user authorization and, for automatic renewal, a refresh token.
RenoP refreshes stored credentials; the email settings do not provide an interactive OAuth callback.
Use your registered application's authorization-code flow and exact registered redirect URI, then save the resulting refresh token privately.

## Cloudflare Email

Enable Email Sending for the account and onboard the sender domain with its required DNS records.
Create a token with **Email Sending: Edit**, restricted to that account; enter it as `api_key` and enter the owning `account_id`.
Email Routing permissions alone do not grant outbound sending. Domain/DNS setup privileges are separate from runtime sending permission.

RenoP calls `POST /accounts/{account_id}/email/sending/send` under `https://api.cloudflare.com/client/v4`.
It submits structured addresses plus text/HTML and reads `message_id`, `delivered`, `permanent_bounces`, `queued`, and `suppressed_recipients`.
Queued results remain `queued_provider`; RenoP does not consume Cloudflare's separate event subscriptions.
See [setup and token permissions](https://developers.cloudflare.com/email-service/get-started/send-emails/) and the [sending schema](https://developers.cloudflare.com/api/resources/email_sending/methods/send/).

## Microsoft Graph

Register an Entra application in the mailbox's cloud. Personal Outlook.com accounts use delegated authorization;
Microsoft 365 organizations can use delegated or application permissions.
For delegated access, authorize `Mail.Send Mail.Read offline_access`, save `client_id`, the applicable `client_secret`, and `refresh_token`, and use `mailbox: me`.
Tenant `common` works for a compatible global application registration; organization policies can require administrator approval.

For application access, grant application `Mail.Send` and `Mail.Read` with administrator consent.
Set the actual tenant ID, client ID, client secret, and target `mailbox` user ID or address.
RenoP requests `client_credentials` with the selected Graph resource's `/.default` scope.
Application tokens cannot use `/me`. Restrict the application's mailbox access through Exchange's supported application access controls.

| Cloud | API base | Token authority |
| --- | --- | --- |
| Global / Outlook.com / Microsoft 365 GCC | `https://graph.microsoft.com/v1.0` | `https://login.microsoftonline.com` |
| US government L4 / GCC High | `https://graph.microsoft.us/v1.0` | `https://login.microsoftonline.us` |
| US government L5 / DoD | `https://dod-graph.microsoft.us/v1.0` | `https://login.microsoftonline.us` |
| China / 21Vianet | `https://microsoftgraph.chinacloudapi.cn/v1.0` | `https://login.chinacloudapi.cn` |

Sending uses `POST /me/sendMail` or `POST /users/{mailbox}/sendMail`, with the configured `from` address and a saved Sent Items copy.
Status queries filter Sent Items by RenoP's extended tracking property; `Mail.ReadBasic` does not cover these properties.
For delegated sending from another mailbox, obtain `Mail.Send.Shared`, appropriate shared read permission, and Exchange Send As/Send on Behalf authority; accessing that mailbox directly also requires Full Access.
HTTP 202 means accepted; finding the saved message means `sent`.
See [sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail), [extended-property permissions](https://learn.microsoft.com/en-us/graph/api/singlevaluelegacyextendedproperty-get?view=graph-rest-1.0),
[shared sending](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user), and [cloud endpoints](https://learn.microsoft.com/en-us/graph/deployments).

## Amazon SES

Verify the sender identity in the selected region and obtain production access when sending outside the SES sandbox.
Configure an IAM access key and secret; temporary credentials also require their session token and administrator renewal before expiration.
SES SMTP credentials are not SES API credentials.

Grant `ses:SendEmail` on the permitted identity resources. Grant `ses:GetAccount` and `ses:GetMessageInsights` with resource `*`, as these lookups do not support identity-level resource restrictions.
Message insights require the applicable Virtual Deliverability Manager feature; its charges are separate from ordinary sending.
RenoP signs requests using AWS SigV4 with service `ses` and the configured region.

`POST /v2/email/outbound-emails` returns `MessageId`; `GET /v2/email/account` supplies `SendingEnabled` and the rolling 24-hour sending quota.
`GET /v2/email/insights/{message_id}/` supplies recipient events. SES hard sending limits remain binding in force-send mode.
See [IAM actions](https://docs.aws.amazon.com/service-authorization/latest/reference/list_sesv2.html),
[account lookup](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetAccount.html),
[message insights](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetMessageInsights.html), and [regional endpoints](https://docs.aws.amazon.com/general/latest/gr/ses.html).

## Twilio SendGrid

Authenticate the sender/domain and create a restricted API key with `mail.send`, `messages.read`, and `user.credits.read` as needed.
Email Activity API access requires the additional history product; a sending-only key or plan is insufficient for polling.
EU sending requires an eligible Pro-or-higher plan, an EU subuser and an EU IP; use that subuser's key with the EU preset.

RenoP calls `POST /mail/send`, `GET /messages`, and `GET /user/credits` below the chosen `/v3` base.
The submission's `X-Message-Id` is a prefix of recipient activity IDs; status matching also checks the exact recipient.
The query uses double-quoted values, for example `msg_id LIKE "submission-id%"`.
Credits are message allowances, not a withdrawable account balance.
See [API-key scopes](https://www.twilio.com/docs/sendgrid/api-reference/api-key-permissions),
[activity access](https://www.twilio.com/docs/sendgrid/api-reference/email-activity/filter-all-messages),
[query syntax](https://www.twilio.com/docs/sendgrid/for-developers/sending-email/getting-started-email-activity-api),
[credits](https://www.twilio.com/docs/sendgrid/api-reference/users-api/retrieve-your-credit-balance), and [regional sending](https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send).

## Google Gmail

Enable the Gmail API in the OAuth client's Google Cloud project and configure its consent screen.
Authorize the mailbox user with `https://www.googleapis.com/auth/gmail.send` and `https://www.googleapis.com/auth/gmail.metadata`.
Request `access_type=offline`; obtain renewed consent if Google does not issue the initial refresh token.
An external application left in Testing can receive refresh tokens that expire after seven days; publishing and restricted-scope verification requirements depend on the application.

Save the client ID, client secret, and refresh token. RenoP exchanges it at `https://oauth2.googleapis.com/token`.
The configured sender must be the authenticated mailbox or an authorized send-as alias.
Service-account JSON keys and domain-wide delegation JWT generation are not implemented by this email connector.

`POST /users/me/messages/send` under `https://gmail.googleapis.com/gmail/v1` accepts base64url MIME and returns a message ID.
RenoP queries `GET /users/me/messages/{message_id}` with `format=minimal` and checks the `SENT` label.
This confirms `sent`, not delivery. Gmail API request quotas are distinct from mailbox sending limits.
See [send scopes](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send),
[metadata access](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/get), and [OAuth setup](https://developers.google.com/identity/protocols/oauth2/web-server).

## Alibaba Cloud Direct Mail

Activate Direct Mail, verify the domain, and configure the sender address in the selected region.
Use a RAM access key/secret with `dm:SingleSendMail`, `dm:DescAccountSummary`, and `dm:SenderStatisticsDetailByParam`; these actions use resource `*`.
For optional balance calibration, grant **`bss:DescribeAcccount`**: the three consecutive `c` characters are the official permission spelling.
The billing action name remains `QueryAccountBalance`, version `2017-12-14`; it is not a `dm:` permission.

RenoP uses signed RPC POST requests with Direct Mail version `2015-11-23`.
`SingleSendMail` returns `EnvId`; `DescAccountSummary` exposes free allowances and account status.
The public delivery-statistics result lacks a reliable per-message ID, so RenoP reports `unknown` after querying instead of matching by recipient alone.
Use a sender alias of at most 15 characters. Choose the billing endpoint for the account's commercial region, independently of the sending region; match the pricing currency to the returned currency.

See [sending and limits](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-singlesendmail),
[quota permission](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-descaccountsummary),
[statistics](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-senderstatisticsdetailbyparam),
[balance authorization](https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-queryaccountbalance), and [endpoints](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint).

## Tencent Cloud SES

Verify the sending domain/address and grant CAM actions `ses:SendEmail` and `ses:GetSendEmailStatus`.
Both operations use resource `*` in the documented CAM policy.
Optional account-balance access uses `finance:DescribeAccountBalance`, although the signed API service is `billing`.
Enter the SecretId as `api_key`, SecretKey as `api_secret`, and temporary Token as `session_token` when applicable.

Ordinary accounts must use an approved provider template. Set `tencent_template_id` to its positive ID.
Create an HTML template with the following text substitutions, submit it for provider approval, then use its ID:

```html
<div style="padding:24px;background:#f3f5f8;font-family:Arial,sans-serif">
  <div style="max-width:600px;margin:auto;padding:24px;background:white;border:1px solid #dfe5ef;border-radius:20px">
    <p style="color:#3158c9;font-weight:bold">RenoP</p>
    <h1>{{subject}}</h1>
    <div style="white-space:pre-wrap;overflow-wrap:anywhere">{{text}}</div>
  </div>
</div>
```

RenoP supplies the subject and complete plain-text notification, escaping HTML characters before substitution.
Variables must appear in text content, never in attributes or scripts. The entire `TemplateData` JSON is limited to 800 UTF-8 bytes; larger notifications fail before submission and are not truncated.
Provider approval is required and is not guaranteed by this example. In template mode the approved provider HTML controls styling.
A zero ID retains `Simple` for historical accounts with explicit custom-content approval; it does not enable that permission for an ordinary account.

`SendEmail` and `GetSendEmailStatus` use version `2020-10-02` and TC3 signing with service `ses`.
Status matches message ID and recipient; provider acceptance, delivery, rejection, and temporary deferral remain distinct.
`DescribeAccountBalance` uses version `2018-07-09`; RenoP converts the available balance from cents to currency millionths.
China and international endpoints use their respective account systems and currencies.
See [SendEmail](https://intl.cloud.tencent.com/document/product/1084/39408),
[template and status fields](https://intl.cloud.tencent.com/document/product/1084/39418),
[SES CAM actions](https://intl.cloud.tencent.com/document/product/598/57150),
[billing permissions](https://cloud.tencent.com/document/product/555/61542), and [balance API](https://cloud.tencent.com/document/api/555/20253).

## Feishu and Lark Mail

Create and publish a custom application for an organization with an active Mail mailbox.
Grant `mail:user_mailbox.message:send`, `mail:user_mailbox.message:readonly`, and `offline_access`.
Make the application available to the sending user, enable token refresh if the application's security settings expose that switch, and obtain the user's consent.
Use a **user** access/refresh token; a tenant token does not authorize the sending and delivery-status operations used here.

Configure the app ID, app secret, refresh token, and mailbox address or `me`.
Feishu and Lark have separate app registrations, credentials, and API bases:
`https://open.feishu.cn/open-apis` and `https://open.larksuite.com/open-apis`.
RenoP currently refreshes through the compatible `POST /authen/v2/oauth/token` interface and persists each rotated refresh token before sending.

`POST /mail/v1/user_mailboxes/{mailbox}/messages/send` accepts padded base64url MIME and returns `data.message_id`.
`GET /mail/v1/user_mailboxes/{mailbox}/messages/{message_id}/send_status` returns recipient details:
4 means delivered; 3 and 6 mean failure; 0, 1, 2, and 5 continue polling.
Only recipient delivery results establish delivered status.
See [sending](https://open.feishu.cn/document/server-docs/mail-v1/user_mailbox-message/send),
[delivery status](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status),
[Lark delivery status](https://open.larksuite.com/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status), and [token renewal](https://open.feishu.cn/document/authentication-management/access-token/refresh-user-access-token).

## SMTP OAuth

For Gmail SMTP, consent to `https://mail.google.com/`; the Gmail API's send-only scope is insufficient for SMTP.
For Outlook.com/Microsoft 365 SMTP, consent to `https://outlook.office.com/SMTP.Send` plus `offline_access`.
SMTP AUTH must also be enabled where the Microsoft 365 organization/mailbox policy requires it.
Set the SMTP username to the mailbox address and configure the corresponding client and refresh credentials.

RenoP's automatic SMTP refresh uses delegated credentials. It does not acquire SMTP application tokens using `SMTP.SendAsApp`.
An externally managed access token can be configured, but its renewal remains the administrator's responsibility.
See [Google SASL OAuth](https://developers.google.com/workspace/gmail/imap/xoauth2-protocol) and [Microsoft SMTP OAuth](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth).

## Verification and Operations

Save the account, send a test message from the administrator interface, and inspect its final job status and calibration result.
Verify receipt at a mailbox you control. A 202 enqueue response tests only queue acceptance.
Missing status permission can produce `unknown` after an accepted send; do not resend merely because the status query failed.
Check the provider code in global logs and grant only the missing capability.

Official-document review and local HTTP contract tests do not prove that your credentials, subscription, sender domain, or regional account work.
This review did not use live provider credentials. Real-provider sending, quota, balance, and delivery checks must be completed with the deployment's own accounts.
Unknown limits use the behavior described in [Email Delivery](mail.md); known limits are retained when calibration fails.
