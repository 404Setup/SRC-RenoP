---
title: Email appearance
order: 8
category: Configuration
description: Template presets, browser preview, and the shared RenoP visual style
---

# Email appearance

Email templates use RenoP's blue action buttons, rounded cards, neutral backgrounds, and system typography.
The site name appears above the message, verification codes have a distinct panel, and additional details are
separated from the main message. Plain-text alternatives contain the same message and action URL.

## Choose a preset

Set `mail.template_style`, or choose the template style on the Email settings page:

```yaml
mail:
  template_style: card
```

- `card`: the standard card with comfortable spacing.
- `compact`: smaller content padding and heading size.
- `notice`: the standard card with an amber top border.

All presets keep the same scene identifiers and account-language selection. No external images, fonts, scripts,
or stylesheets are required. Inline colors provide the base appearance; responsive and dark-mode rules apply when
the recipient's client supports them. User text remains escaped, and action URLs must belong to the configured instance.

## Preview and delivery

Administrators can preview a scene from Email settings without sending a message.
GET /api/settings/mail/templates/:scene?style=card returns its subject, HTML, and plain text. Preview language follows
the request's `Accept-Language`; delivered account notifications use the recipient's stored language, with English
as the fallback. The preview uses sample values and does not contain stored credentials.

Template changes apply when a new message enters the sending queue. An already queued message keeps the rendered
content stored with that job. Delivery limits, credit reservations, status checks, and the rule against retrying
an interrupted submission remain unchanged. See [Email delivery](mail.md) for provider and queue configuration.
