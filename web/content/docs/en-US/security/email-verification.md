---
title: Security Email Verification
order: 5
category: Security
description: Verify a private account email using a delivered code or GitHub
---

# Security Email Verification

## Change an address

Edit the private email in **Account security**. When email delivery is enabled, saving queues a verification message for the new address. Enter its eight-digit code to apply the change. The current address continues to work until verification succeeds. When delivery is disabled, saving applies the address directly.

The configured email blacklist or whitelist applies to both methods, including when delivery is disabled. Rules match an exact mailbox or domain; subdomains are separate. Unicode and Punycode spellings of the same internationalized domain match the same rule. An address already owned or still reserved by a retired account cannot be claimed.

## Verification API

These operations require the current browser cookie. JSON bodies require `Content-Type: application/json` and are limited to 4,096 bytes.

| Method | Path | Request or response |
|---|---|---|
| GET | `/api/auth/profile/security` | Includes `email` and `email_verification_required` |
| PUT | `/api/auth/profile/email` | `{"email":"new@example.com"}`; `202` with `{id,status,ticket}` when queued, otherwise `200` with account security |
| POST | `/api/auth/profile/email/confirm` | Email and code; `200` with updated account security |
| GET | `/api/auth/mail/:id` | Delivery status; the requesting account or `X-Renop-Mail-Ticket` authorizes access |

```json
{"email":"new@example.com","code":"01234567"}
```

Codes expire after 10 minutes, allow five wrong attempts, and belong to the browser session that requested them. A new code can be issued after 60 seconds, subject to the configured IP sending limit. A successfully queued replacement invalidates the previous code. Failed queue insertion preserves it. Code storage, queue insertion, and IP rate accounting commit together.

An incorrect, expired, spent, or stale code returns `400` with `ACCOUNT_EMAIL_CODE_INVALID`; the browser remains signed in. An occupied address returns `409` with `ACCOUNT_EMAIL_CONFLICT`. Recipient-policy failures return `400` with `mail_recipient_blocked`. A revoked session cannot confirm a change. Changes to account credentials or security settings invalidate pending proofs.

## GitHub verification

When GitHub login is configured, **Use verified GitHub email** authorizes a fresh lookup through `GET /api/auth/github/start?intent=email`. This flow requests `user:email` and reads the [authenticated GitHub email list](https://docs.github.com/en/rest/users/emails#list-email-addresses-for-the-authenticated-user).

RenoP selects a verified primary contact address, or the first verified contact address when no primary address is available. GitHub no-reply addresses are excluded. The resulting address is checked against the local recipient policy and saved. This works without a configured email sender and does not create or replace a GitHub login binding.

The callback is single-use, expires after 10 minutes, and must return to the initiating account and browser session with unchanged credentials. Provider access tokens are used only for this lookup and are not retained.

## Operation

RenoP retains at most 2,048 pending email changes. Expired records are removed before insertion and by mail cleanup. Verification codes are stored as keyed hashes; queued message contents are encrypted using the private mail encryption key. Codes and delivery tickets stay out of URLs, logs, and browser storage.

Successful changes write a profile activity entry. The serial mail worker sends an `email_changed` notification through the configured routing and sending limits. Queue acceptance is distinct from delivery; the verification dialog displays provider delivery progress and failures.
