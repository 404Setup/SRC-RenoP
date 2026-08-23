---
title: Audit Logs & Message Center
order: 3
category: Security
description: Security audit trails and user notification dispatch
---

# Audit Logs & Message Center

RenoP records security-relevant events and provides a durable in-app message center for workflow notifications.

## 1. Security Audit Logs

Sensitive administrative actions and authentication events are logged to the database:

### Tracked Event Types

- Successful and failed login attempts
- Password changes and credential resets
- User creation, role modifications, and deletions
- Access token creation and revocations
- Repository configuration changes
- System updates and configuration reloads

Audit logs can be filtered by timestamp and action via the Web UI or API.

---

## 2. User Message Center

The message center dispatches notifications to users inside the Web console.

### Notification Categories

1. **System Announcements**: Broadcast messages from administrators.
2. **Workflow Events**: Build completion notices, GPG signature verification results.
3. **Collaboration**: Repository invitations and permission grant notifications.

Users receive an unread message badge in the top navigation bar and can mark individual or all messages as read.
