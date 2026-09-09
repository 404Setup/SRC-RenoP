---
title: Security Email Verification
order: 5
category: Security
description: Verify a private account email using a delivered code or GitHub
---

# Security Email Verification

## Change an address

Edit the private email in **Account security**. When email delivery is enabled, saving queues a verification message for
the new address. Enter its eight-digit code to apply the change. The current address continues to work until
verification succeeds. When delivery is disabled, saving applies the address directly.

The configured email blacklist or whitelist applies to both methods, including when delivery is disabled. Rules match a
complete mailbox, an exact @domain provider, or a .suffix including subdomains. Blacklist mode can also use the built-in
temporary-email list; whitelist mode ignores that list. Unicode and Punycode spellings of the same internationalized
domain match the same rule. An address already owned or still reserved by a retired account cannot be claimed.

## Login aliases

Primary and secondary email addresses share one account ownership record. A provider binding fails if the identity or
any returned contact address belongs to another account, including an address reserved after retirement. Binding, email
reservations, and account-security changes commit together; failure leaves the existing account unchanged. Emails never
merge accounts.

**Other login emails** lists private secondary addresses. They identify the same account for password or Passkey login,
email password reset, and offline-code recovery; existing password and second-factor policies still apply. Notifications
continue to use the primary email. Each account can retain up to 128 addresses, including its primary email.

A provider must verify a returned address, or that address must already belong to the signed-in account. Otherwise,
verify it with the provider or use **Add a verified email** before connecting. Adding an address requires RenoP email
delivery and the existing code dialog. Provider registration with an unverified email must confirm that provider
address; confirming a different address is insufficient. GitHub no-reply addresses are excluded.

Disconnecting a provider keeps its email aliases. Removing an alias releases that address; a later provider
authorization may add it again. The primary address cannot be removed as an alias. Bans preserve all addresses, and
retirement reserves all of them for 14 days, even when there is no primary address. Administrator early release applies
to the entire retained address set.

| Method | Path                            | Request or response                                                             |
|--------|---------------------------------|---------------------------------------------------------------------------------|
| GET    | `/api/auth/profile/security`    | Adds `email_aliases` alongside `email`                                          |
| PUT    | `/api/auth/profile/email`       | `{"email":"alias@example.com","alias":true}`; `202` with a verification receipt |
| DELETE | `/api/auth/profile/email/alias` | `{"email":"alias@example.com"}`; updated account security                       |

Alias confirmation uses the existing confirmation endpoint; its purpose is saved with the challenge and cannot be
changed by the confirmation request. Alias removal requires a browser sign-in from the last five minutes. Errors use
`ACCOUNT_EMAIL_PROOF_REQUIRED`, `ACCOUNT_EMAIL_LIMIT`, or `ACCOUNT_EMAIL_PRIMARY` with `409`; an older removal session
receives `MFA_REAUTH_REQUIRED`.

Upgrades backfill existing primary email ownership. Previously linked providers acquire aliases on their next
authorization because older installations did not retain those addresses. Session persistence rechecks bans and account
expiry inside the account transaction before issuing a new browser session.

## Verification API

These operations require the current browser cookie. JSON bodies require `Content-Type: application/json` and are
limited to 4,096 bytes.

| Method | Path                              | Request or response                                                                                               |
|--------|-----------------------------------|-------------------------------------------------------------------------------------------------------------------|
| GET    | `/api/auth/profile/security`      | Includes `email` and `email_verification_required`                                                                |
| PUT    | `/api/auth/profile/email`         | `{"email":"new@example.com"}`; `202` with `{id,status,ticket}` when queued, otherwise `200` with account security |
| POST   | `/api/auth/profile/email/confirm` | Email and code; `200` with updated account security                                                               |
| GET    | `/api/auth/mail/:id`              | Delivery status; the requesting account or `X-Renop-Mail-Ticket` authorizes access                                |

```json
{"email":"new@example.com","code":"01234567"}
```

Codes expire after 10 minutes, allow five wrong attempts, and belong to the browser session that requested them. A new
code can be issued after 60 seconds, subject to the configured IP sending limit. A successfully queued replacement
invalidates the previous code. Failed queue insertion preserves it. Code storage, queue insertion, and IP rate
accounting commit together.

An incorrect, expired, spent, or stale code returns `400` with `ACCOUNT_EMAIL_CODE_INVALID`; the browser remains signed
in. An occupied address returns `409` with `ACCOUNT_EMAIL_CONFLICT`. Recipient-policy failures return `400` with
`mail_recipient_blocked`. A revoked session cannot confirm a change. Changes to account credentials or security settings
invalidate pending proofs.

## GitHub verification

When GitHub login is configured, **Use verified GitHub email** authorizes a fresh lookup through
`GET /api/auth/github/start?intent=email`. This flow requests `user:email` and reads
the [authenticated GitHub email list](https://docs.github.com/en/rest/users/emails#list-email-addresses-for-the-authenticated-user).

RenoP selects a verified primary contact address, or the first verified contact address when no primary address is
available. GitHub no-reply addresses are excluded. The resulting address is checked against the local recipient policy
and saved. This works without a configured email sender and does not create or replace a GitHub login binding.

The callback is single-use, expires after 10 minutes, and must return to the initiating account and browser session with
unchanged credentials. Provider access tokens are used only for this lookup and are not retained.

## Operation

RenoP retains at most 2,048 pending email changes. Expired records are removed before insertion and by mail cleanup.
Verification codes are stored as keyed hashes; queued message contents are encrypted using the private mail encryption
key. Codes and delivery tickets stay out of URLs, logs, and browser storage.

Successful changes write a profile activity entry. The serial mail worker sends an `email_changed` notification through
the configured routing and sending limits. Queue acceptance is distinct from delivery; the verification dialog displays
provider delivery progress and failures.

Other providers may offer [Use verified email](./oauth-login.md) in the profile editor. This also requests fresh
authorization, preserves the current login binding, and applies the recipient policy. An address without an explicit
verified-email claim must use RenoP email verification instead.

## Account language

Signing in restores the account's saved language on each device. An account without a preference adopts the current
interface language; later language-picker changes are saved automatically. Registration saves the page language with the
new account. Failed writes keep one pending choice on the device and retry after reconnection, returning to the page, or
reloading. A previous account's pending choice is never applied to another account.

The preference is private, survives username changes and restarts, and is removed when the account is retired. Updating
it does not alter credentials or invalidate an active second-factor challenge. Background mail and password-reset emails
use the recipient's preference, including when an email alias identifies the account. Recipients without a preference
use the initiating page's `Accept-Language`, then `en-US`. Queued messages keep the language chosen at enqueue time.

Supported identifiers are `en-US`, `zh-CN`, `zh-HK`, `zh-TW`, `zh-YUE`, `ko-KR`, `ja-JP`, `de-DE`, `fr-FR`, `ru-RU`,
`es-ES`, and `pt-PT`.

| Method | Path                       | Request or response                                                                                             |
|--------|----------------------------|-----------------------------------------------------------------------------------------------------------------|
| GET    | `/api/auth/profile/locale` | `{"user_id":"00000000-0000-4000-8000-000000000001","locale":"fr-FR"}`; an empty value means no saved preference |
| PUT    | `/api/auth/profile/locale` | `{"user_id":"00000000-0000-4000-8000-000000000001","locale":"fr-FR"}`; returns the saved canonical identifier   |

Both operations require the current browser cookie and return `Cache-Control: no-store` for preference data. API tokens
cannot read or change this preference. Unsupported values return `400` with
`ACCOUNT_LOCALE_INVALID`. [Mail configuration](../configuration/mail.md) no longer offers a global language selector.

PUT must echo the immutable `user_id` returned by GET; a mismatched account returns `403`. Local pending choices are
also bound to this ID, so renaming or later reusing a username cannot transfer preferences between accounts.
