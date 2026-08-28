---
title: Audit Logs & Message Center
order: 3
category: Security
description: Durable behavior records, workflow notifications, and privacy boundaries
---

# Audit Logs & Message Center

Audit logs and user messages have different purposes. Audit records answer who performed a security-relevant action;
messages present a localized result or workflow to the affected user. Both are durable database data.

## Audit logs

Audit writes use stable action identifiers from one backend registry. Frontend validation requires every registered
action to have a translation in every supported locale.

### Recorded events

- login success/failure, password changes, recovery, and login-method changes;
- API Token creation/revocation and session revocation;
- user, role, repository, storage, proxy, and update administration;
- Maven domain verification/team changes and npm/Cargo/Docker package-team lifecycle;
- uploads, deletes, GPG quarantine/publication, and other package mutations.

Entries include subject, operator where applicable, authentication method, session public ID, client IP, time, and a
bounded detail string. Retention and maximum rows are configured globally. Only authorized users can view or clear logs.

## Message center

Messages support pagination, unread counts, per-message/all-read operations, deletion, and pending workflow actions.

### Message categories and privacy

- **Announcements**: Administrator messages to selected or all accounts.
- **Workflow**: Team invitations, GPG outcomes, and other actions requiring a decision.
- **Collaboration**: Package/domain membership changes and neutral removal notices.
- **System results**: Update availability and durable failures; transient download/restart progress remains a toast.

A team-removal message identifies the repository and package or Maven domain but deliberately omits the operator.
Update and workflow messages use dedupe keys so repeated checks do not flood the inbox. The unread count appears both in
the account menu and next to the navigation avatar.
