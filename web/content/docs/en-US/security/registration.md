---
title: Account Registration
order: 6
category: Security
description: Enable registration, email confirmation, and GitHub account creation
---

# Account Registration

## Registration settings

Registration is disabled by default. An administrator can enable it in the service settings. Disabling it makes `/account/register` and registration operations return `404`; the public status endpoint remains available. The following top-level configuration uses the defaults:

```yaml
registration:
  enabled: false
  ip_limit: 1
  ip_interval: {value: 3, unit: week}
  provider_cooldown: {value: 12, unit: hour}
```

Only successful registrations consume the IP allowance. The default is one account per three weeks, starting with the first successful registration. Equivalent IPv4 and IPv6 spellings share the same limit. Retirement does not restore this allowance. Limits persist across restarts. Counts must be 1–10,000; intervals must be positive, use `minute`, `hour`, `day`, `week`, or `month`, and span at most 365 days. A month is 30 days.

## Create an account

Open the registration page from sign-in. Usernames contain 4–18 ASCII letters, digits, or underscores and are stored in lowercase. Nicknames are optional and allow 36 Unicode characters. Passwords are mandatory and contain 6–72 UTF-8 bytes.

When mail is enabled, an email address and its eight-digit verification code are required. Codes expire after ten minutes and allow five incorrect attempts. Keep the same browser and IP address while confirming. Delivery uses the existing serial mail queue, recipient policy, quota, and rate limits. The page shows delivery status. For manual registration without mail, the email is optional. After success, sign in with the new credentials; a registration-success message is queued when mail is enabled.

## Register through GitHub

An unlinked GitHub login opens the confirmation page. RenoP requests `read:user read:org user:email`, selects a verified real contact address, and excludes no-reply addresses. No email code is needed. Authorization is bound to a ten-minute HttpOnly browser cookie, and each callback state can be used once.

Confirm within ten minutes and set a password. No usable account exists before confirmation. Expiry removes the pending personal data and prevents the same GitHub identity from starting again for the configured cooldown, twelve hours by default after expiry. Choose whether to import the username, nickname, and avatar. Local name limits apply; an unavailable username must be entered manually. Missing optional data or an avatar that exceeds size or quota limits does not prevent registration.

## Registration API

Public JSON requests require `Content-Type: application/json` and are limited to 4,096 bytes. Confirmation also uses the private HttpOnly registration cookie; never store a password, code, or mail ticket in browser storage.

- `GET /api/auth/registration/status`: Read availability and email requirements.
- `GET /api/auth/registration/pending`: Read this browser's pending confirmation.
- `POST /api/auth/registration/code`: Queue a code; returns `202` with a private mail receipt.
- `POST /api/auth/registration`: Confirm registration.
- `GET /api/settings/registration`, `PUT /api/settings/registration`: Read or update registration settings (administrator).

```json
{
  "username": "new_user",
  "nickname": "New User",
  "email": "user@example.com",
  "password": "a unique long password",
  "code": "12345678"
}
```

Use `provider: "github"` when confirming a pending GitHub registration; keep its verified email and omit `code`. Set `import_avatar` to `true` to request avatar import. Success returns `201` with `username` and `avatar_imported`, without creating a login session. Conflicts return `409`; expired or invalid confirmation returns `400`; exhausted IP limits or provider cooldown return `429`. Account, email, provider binding, IP accounting, and confirmation consumption commit together.

Other configured services follow the [third-party registration rules](./oauth-login.md): send their provider ID for both code issuance and confirmation. A missing or unverified provider email always requires a RenoP verification code; this registration cannot complete while mail is unavailable. Verified provider emails need no additional code.
