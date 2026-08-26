---
title: Audit log と message center
order: 3
category: セキュリティ
description: Durable behavior record、workflow notification、privacy boundary
---

# Audit log と message center

audit と user message は目的が異なります。audit は誰が security action を行ったかを記録し、message は影響を
受ける user へ localized result/workflow を示します。どちらも DB に durable 保存します。

## Audit log

backend の単一 registry にある stable action ID を使用します。frontend validation は全 action の全 locale
translation を要求します。

### 記録する event

- login、password、recovery、login-method change
- API Token create/revoke と session revoke
- user、role、repository、storage、proxy、update administration
- Maven domain verification/team と Cargo/Docker team lifecycle
- upload、delete、GPG quarantine/publication、package mutation

entry は subject、必要な operator、auth method、session public ID、client IP、time、bounded detail を含みます。
retention/max rows は global 設定で、許可 user だけが read/clear できます。

## Message center

pagination、unread count、individual/all read、delete、pending workflow action に対応します。

### Category と privacy

- **Announcement**: 選択または全 account への administrator message。
- **Workflow**: team invitation、GPG result、decision action。
- **Collaboration**: membership change と neutral removal notice。
- **System result**: update availability と durable failure。一時 progress は Toast。

team removal は repository と package/Maven domain を示しますが operator は意図的に省略します。dedupe key が
反復 check の flood を防ぎ、unread count は account menu と navigation avatar の両方に表示します。
