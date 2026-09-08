---
title: Email Delivery
order: 6
category: Configuration
description: Email providers, durable delivery, quotas, billing, templates, and administrator APIs
---

# Email Delivery

Configure email under **Settings → Service**. Add a sending account, choose a provider preset, enter its credentials, and save.
Enable email after setting the public HTTPS URL. Changes apply to subsequent queue operations without restarting RenoP.

## Configuration

```yaml
mail:
  enabled: false
  public_url: https://packages.example.com
  site_name: RenoP
  template_style: card
  locale: en-US
  delay: {value: 5, unit: second}
  manual_rate: {limit: 1, interval: {value: 2, unit: minute}}
  account_rate: {limit: 50, interval: {value: 1, unit: minute}}
  calibration: {value: 5, unit: minute}
  list_mode: blacklist
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

One enabled account handles every scene. With several enabled accounts, explicit scene assignments take priority over the single `*` fallback.
Two enabled accounts cannot claim the same scene. An unassigned scene has no sender; RenoP does not silently choose another account.
Each account has a stable `id`; editing its display name preserves counters. Removing an account cancels its unsent messages when the worker next processes them.
Disabling email or an account pauses sending. Existing messages retain their expiration times.

`delay` accepts seconds, minutes, or hours; zero removes the delay while preserving serial delivery.
`manual_rate` applies to manual requests per IP, including test emails; its interval accepts minutes, hours, or days.
`account_rate` includes manual and automatic attempts per sending account; its interval also accepts seconds.
Both rate limits and their interval values must be positive. The defaults are one manual request per two minutes and 50 attempts per account per minute.

`list_mode` is `blacklist` or `whitelist`. Entries in `addresses` are exact mailboxes or `@example.com` domains.
Domain matching does not include subdomains. The policy is checked both before queueing and immediately before sending.

## Providers and Endpoints

| Provider | Presets and credentials | Remote capabilities |
| --- | --- | --- |
| SMTP | Plain SMTP, implicit SSL/TLS, or required STARTTLS; username/password or supported OAuth | Local estimates; SMTP acceptance response |
| Cloudflare Email | Account ID and API token; global REST endpoint | Initial delivered, rejected, or queued result |
| Microsoft Graph | Outlook.com and Microsoft 365/Entra; global, US government L4/L5, and China endpoints | Sent-folder status |
| Amazon SES | Regional IPv4, dual-stack, and available FIPS endpoints; access key, secret, optional session token | Sending quota and message insights |
| Twilio SendGrid | Global and EU endpoints; API key | Credits and Email Activity status |
| Google Gmail | Gmail and Workspace; delegated OAuth credentials | Sent-label status |
| Alibaba Cloud Direct Mail | Hangzhou, Singapore, Virginia, Frankfurt; public and VPC endpoints; access key and secret | Free quota, account balance, uncorrelated delivery statistics |
| Tencent Cloud SES | China and international API/billing endpoints; access key and secret | Account balance and recipient delivery status |
| Feishu / Lark Mail | Feishu and Lark endpoints; delegated user OAuth credentials | Sent-message status |

The [SES endpoint catalog](https://docs.aws.amazon.com/general/latest/gr/ses.html) and [Direct Mail endpoint catalog](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint) define regional availability.
Direct Mail VPC endpoints require connectivity within the corresponding region. Decommissioned Sydney endpoints are excluded.
SMTP presets include Gmail, Outlook.com, Microsoft 365, QQ, NetEase 163/126, Cloudflare, SendGrid, SES, Direct Mail, Tencent, and Feishu.
Endpoint and pricing fields remain editable. Changing a preset loads its endpoint and price defaults; changing provider does not reuse another provider's stored credentials.

Manual quota and overage limits remain configured. A preset currency change clears the manual balance instead of reinterpreting its units.

## Credentials and Permissions

SMTP `smtp_security` is `plain`, `tls`, or `starttls`; `smtp_port` defaults to 465 for implicit TLS and 587 otherwise.
TLS validates the certificate and server name. STARTTLS is required when selected. Plain SMTP must be selected explicitly.
Use an application password when required by the mailbox provider. OAuth SMTP supports Gmail and Microsoft refresh credentials; access-token-only configurations require token renewal by the administrator.

Graph delegated accounts use `client_id`, optional `client_secret`, and `refresh_token`; personal accounts can use tenant `common`.
Application access uses an Entra tenant ID and client credentials. Set `mailbox` to the user ID or email address for application access; delegated access can use `me`.
Grant `Mail.Send` for sending and the necessary `Mail.Read` permission for sent-folder queries; application permissions require administrator consent.
See [Graph sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail) and [national cloud deployments](https://learn.microsoft.com/en-us/graph/deployments).

Gmail requires delegated sending and message-metadata access. Feishu/Lark requires a user access token, with message sending and read permissions.
RenoP refreshes supported OAuth tokens serially and persists rotated refresh tokens before sending.
API-specific fields include `api_key`, `api_secret`, optional `session_token`, `account_id`, `region`, `endpoint`, and optional `billing_endpoint`.
The configured sender must be authorized by the provider. Account IDs and mailbox IDs are not interchangeable with API keys.

## Quotas and Billing

`quota.limit` is the local sending allowance: negative means unlimited, zero or omitted means automatic discovery, and a positive value selects a manual allowance.
If no remote quota is available, automatic mode starts without a quota limit. Failed calibration retains previously known limits instead of granting new credits.
Manual quota periods are UTC calendar hours, days, weeks starting Monday, or months. Usage is retained independently for all four periods.

Enable `force_send` to use the overage allowance after the regular quota is exhausted.
`overage.limit` is negative for unlimited overage and zero to prohibit it; its period also accepts `hour`, `day`, `week`, or `month`.
Provider hard limits still apply. SES sending limits are technical limits and cannot be bypassed by paying for overage.
Where quota APIs are unavailable, enter the allowance included in your purchased plan manually.

`pricing.tiers` contains ascending cumulative `up_to` boundaries. Only the final boundary can be zero, meaning unlimited volume.
Each tier charges `amount_micros` for `batch_size` messages. Use a batch size of one for individual-message prices.
`rounding: proportional` apportions the batch price; `batch` charges a complete batch when its first message is used.
Prices and balances use millionths of the three-letter `currency`: 1000000 means one currency unit. The interface displays ordinary currency amounts.

Presets are dated 2026-09-08: [Cloudflare](https://developers.cloudflare.com/email-service/platform/pricing/) uses USD 0.35 per 1000 overage messages; [SES](https://aws.amazon.com/ses/pricing/) uses USD 0.10 per 1000 outgoing messages.
[SendGrid's published plans](https://sendgrid.com/content/dam/sendgrid/global/en/other/sendgrid-pricing/twi121--sendgrid-pricing-pdf-st1.pdf) provide editable Essentials 50K and Pro 100K overage defaults; EU sending requires an eligible plan.
[Direct Mail](https://www.alibabacloud.com/help/en/direct-mail/billing-methods) uses USD 0.29 per 1000; [Tencent China](https://cloud.tencent.com/document/product/1288/47930) uses CNY 0.0019 per message and [Tencent international](https://www-sg.tencentcloud.com/document/product/1084/39335) uses USD 0.00028 per message.
Graph, Gmail, and Feishu/Lark presets have zero marginal overage estimates; their plan limits remain applicable.
These estimates exclude subscriptions, taxes, attachment data, and optional provider services. Match the editable model to your actual contract.

An empty `balance_micros` means unknown. A known balance at or below zero pauses sending, as does insufficient balance for the next estimated overage charge.
Optional `fetch_balance` calibrates supported provider balances in the configured currency. An unavailable balance does not create a zero balance.
Quota and overage cost are reserved before sending. Invalid credentials, invalid sender configuration, and connections that fail before submission do not consume quota.
Other attempted sends consume quota, including recipient rejection and indeterminate submissions. Known nonchargeable failures refund once; attempt-rate limits still count them.
Later delivery checks do not add remote credits again after a newer calibration has already included the provider's adjustment.

`calibration` defaults to five minutes and accepts minutes, hours, or days. Empty, zero, or negative disables remote quota/balance requests.
RenoP deducts locally between calibrations. Enabling remote calibration performs an initial lookup before the account's first send.
No supported public balance or quota API is invented for providers that expose neither.

## Queue and Delivery Status

Sending, OAuth refresh, remote calibration, and delivery lookups share one serial worker. The default delay is five seconds between completed send attempts.
The queue and its account counters survive a restart. A database lease prevents competing workers from submitting simultaneously.
A submission interrupted before its final outcome is persisted becomes `unknown`; it is never resent automatically.

`accepted` means the provider accepted the request. `sent` means a sent-folder or message-state check succeeded.
`delivered` requires an explicit delivery result. Queued, paused, checking, failed, expired, cancelled, and unknown outcomes remain distinct.
SES and SendGrid status lookup requires the corresponding provider feature and permissions. Graph, Gmail, and Feishu sent status does not prove recipient delivery.
Direct Mail's public statistics response lacks message IDs; RenoP queries it but reports `unknown` rather than attributing another message's outcome.
Cloudflare returns its initial result directly; no unsupported per-message polling endpoint is assumed.

Lookups run after submission with bounded backoff. Repeated lookup failure or expiration produces an unknown result and a global log entry.
Provider failure codes are retained in administrator logs. Public job status omits recipients, message contents, credentials, and raw diagnostics.

## Templates and Notifications

`template_style` selects `card`, `compact`, or `notice`. Email copy is available in English and Simplified Chinese, selected by `locale`.
All styles include matching plain text, escaped substitutions, and HTTPS action links restricted to the configured instance.

```text
registration_verify, registration_success, password_reset, password_changed,
email_verify, email_changed, quota_changed, review_status, review_requested,
permission_changed, account_banned, account_unbanned, collaboration_invitation,
super_team_invitation, pending_reviews, unusual_login, security_changed,
account_retired, notification, test
```

Password and Passkey changes, permission updates, bans/unbans, user quota overrides, review events, invitations, and inbox notifications use durable events.
Notifications are deduplicated and do not replay the old audit history when email is first enabled.
New-network login notifications compare the preceding login's IPv4 /24 or IPv6 /56 network; this is a network change signal, not geographic location.
Account-dependent messages recheck the live account and current security email before sending. Retirement removes that account's queued messages.

## Administration API

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

Settings endpoints require administrator authority. JSON replaces the mail configuration; the request body is limited to 1 MiB.
GET and successful PUT omit credential values and return `secrets_configured`, mapping account IDs to configured credential field names.
For the same provider, empty credential strings preserve stored values. Use `clear_secrets: {"primary":["password"]}` to clear a stored credential explicitly.
The seven write-only fields are `password`, `api_key`, `api_secret`, `session_token`, `client_secret`, `access_token`, and `refresh_token`.
The durable encryption key is never accepted or returned through this API.

The test endpoint uses saved settings and accepts the following JSON:

```json
{"account_id":"primary","to":"receiver@example.com"}
```

A successful enqueue returns HTTP 202, which does not establish delivery:

```json
{"id":"opaque-job-id","status":"queued","ticket":"private-status-capability"}
```

Poll the auth status endpoint with the returned `X-Renop-Mail-Ticket` header, or use the owning account's authenticated session.
Keep the ticket private and out of URLs. Invalid ownership or tickets return 404; unavailable storage returns 503.
Account status includes period usage, attempt counts, quota estimates, balance, estimated overage cost, and the latest calibration result.
Jobs support `limit` 1–50 and `offset` 0–10000, with an optional exact `status` filter. Template previews return subject, HTML, and text.

For SMTP, stored credentials are preserved only while the host and username remain unchanged.

## Storage and Limits

At most 64 accounts, 1000 recipient-list entries, and 20 pricing tiers per account are accepted.
Intervals are bounded to one year; quotas and tier volumes are bounded to one billion messages.
Rendered text plus HTML and each provider response are limited to 128 KiB; each job has one recipient and at most a 24-hour lifetime.
The queue allows 2048 active jobs and at most 12048 records overall. Maintenance trims completed history toward 8000 rows and removes records older than seven days.
Unused IP counters expire, and removed sending-account state is cleaned after 24 hours.

Queue payloads and rotating credentials are encrypted with the persistent `mail.encryption_key` stored in the private configuration file.
Back up this key together with the database. Losing or replacing it prevents decryption of pending messages and account state.
Jobs finalized by the worker discard HTML/text content. Interrupted submissions retain their encrypted payload until history cleanup.
Logs and status APIs do not expose message bodies or recipient addresses.
