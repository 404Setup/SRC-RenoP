---
title: Legal documents and cookie choices
order: 15
category: Configuration
description: Configure policy pages, account-entry consent, and browser preferences
---

# Legal documents and cookie choices

## Policy pages

Administrators edit the privacy policy, terms of service, and legal notice on the Legal documents settings page. All three use the same Markdown editor and safe preview; each document is limited to 512 KiB of UTF-8 text. Empty content restores a placeholder. Replace the placeholders with your instance documents.

The documents are stored under `legal` in `config.yaml` and apply immediately after saving. The old privacy file is no longer read: copy its content into settings before upgrading. The old external legal-notice URL is also retired. Existing files are preserved.

Public pages are `/privacy-policy`, `/terms-of-service`, and `/legal-notice`. They remain readable with expired credentials. `GET /api/legal` returns the current privacy/terms revision and `cookie_banner`; `GET /api/legal/:document` returns bounded plain text. `GET /api/privacy-policy` remains an alias.

`GET /api/settings/legal` and `PUT /api/settings/legal` require settings administrator authority and use JSON fields `privacy_policy`, `terms_of_service`, `legal_notice`, and `cookie_banner`.

## Account entry

Login and registration require explicitly accepting the current privacy policy and terms. This includes password, Passkey, provider sign-in, and second-factor completion. Password authentication by package clients keeps its existing protocol rules.

Clients obtain the revision from `GET /api/legal` and acknowledge it through `X-Renop-Legal-Revision` or the browser consent cookie. Missing or stale acceptance returns HTTP 428 with `X-Renop-Error-Code: legal_consent_required`. Successful account entry records the accepted revision in the existing audit event.

## Cookie choices

The floating notice offers necessary-only, accept-all, and category preferences. Necessary cookies support sessions and security; optional third-party verification requires an explicit choice. Footer preferences can always reopen the dialog, including when the notice is disabled.

Category choices stay in browser storage for one year and are invalidated when the privacy policy or terms change. Declining optional services remains valid; integrations must not load them until consent is granted.
