---
title: Security verification
order: 16
category: Security
description: Configure CAPTCHA providers and protected browser actions
---

# Security verification

## Provider and scopes

Configure CAPTCHA in the Security verification settings page. Available modes are disabled, reCAPTCHA v2 checkbox, reCAPTCHA v2 invisible, reCAPTCHA v3, Cloudflare Turnstile, hCaptcha, and Friendly Captcha v2. Use the matching site key and secret/API key from the provider.

Choose password login, registration, manual email, global-team creation, publishing-domain creation, and package creation independently. Passkey and provider login do not use the password-login switch. A registration email request uses manual-mail verification when enabled, otherwise registration verification. Completing registration is a separate protected action.

Only interactive browser/anonymous actions require challenges. Valid API-token and protocol-password credentials remain exempt; this is determined by verified server-side credentials, never User-Agent. Maven browser uploads and chunked-upload initialization check new catalog packages. Existing packages and authorized automated publishing retain their normal permissions.

Secrets are write-only. An unchanged provider and site key preserve a blank secret; changing either, including disabling the provider, clears the previous secret. reCAPTCHA v3 defaults to a minimum score of 0.5. Configure allowed hostnames in Server domains as well as the provider console. Friendly Captcha supports global and EU endpoints.

Hostname checks apply to reCAPTCHA and Turnstile. hCaptcha verifies the expected site key because its hostname field is statistical; Friendly Captcha verifies the site key and checks the origin when the provider supplies one.

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

## Browser verification

Third-party code loads in an isolated widget frame only after the browser permits optional verification services in Cookie preferences. Cancellation, navigation, and consent withdrawal remove the frame. The provider must validate the result successfully; errors, low scores, wrong actions, and wrong hostnames are rejected.

The browser retries once only after an explicit CAPTCHA-required response emitted before the operation starts. Network failures, email submission uncertainty, and other errors do not trigger automatic replay.

## API contract

`GET /api/captcha` returns public provider settings and sets the necessary HttpOnly nonce cookie. `POST /api/captcha/verify` accepts JSON fields `scope`, `provider`, `site_key`, and `response`; responses are limited to 16 KiB and the JSON request to 32 KiB. Successful verification returns a single-use `proof`, valid for 120 seconds.

Send the proof in `X-Renop-Captcha` with the same browser cookies when retrying the protected operation. It is bound to the scope, browser session, and current provider configuration. Missing verification returns `428` with `X-Renop-Error-Code: captcha_required` and `X-Renop-Captcha-Scope`; invalid/replayed proofs return `400`. Provider failures return `503`.

`GET /api/settings/captcha` and `PUT /api/settings/captcha` require settings administration authority and use JSON. Responses never return the secret. Settings apply immediately; changing the private configuration invalidates outstanding approvals.

[reCAPTCHA](https://developers.google.com/recaptcha/docs/verify) · [reCAPTCHA v3](https://developers.google.com/recaptcha/docs/v3) · [Turnstile](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/) · [hCaptcha](https://docs.hcaptcha.com/) · [Friendly Captcha](https://developer.friendlycaptcha.com/docs/v2/getting-started/verify)
