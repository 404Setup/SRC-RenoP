---
title: Global Teams
order: 12
category: API Reference
description: Immutable shared prefixes, T1-T4 membership, invitations, and account limits
---

# Global Teams

Global teams are instance-wide collaboration identities. A team owns one immutable prefix that package engines can
reference without copying its members into package-level member lists. Team records and memberships use immutable
account IDs internally; responses expose usernames only.

## Roles and ownership

Roles are cumulative. T1 provides read access according to package visibility. T2 publishes and maintains versions,
and may request a new npm package or Docker image for the team. A T3 or T4 member must approve that request. T3
manages T1/T2 members and may create packages directly. T4 owns team metadata and can grant T3/T4.

At least one T4 owner must remain. T3 cannot modify another T3 or T4, and cannot grant either role. System
administrators can manage every team without joining it, but account membership limits still apply when they add a
member. Adding the administrator's own account does not generate a redundant message.

## Package and domain bindings

GET /api/super-teams/eligible defaults to teams where the caller has at least T3; `minimum_role` may request T1-T4.
The npm and Docker creation dialogs request T2 so eligible members can submit an approval task. Docker images whose
names contain a slash must select the team matching the first path component. Scoped npm packages must select the team
matching their scope without `@`. Unprefixed Docker images and unscoped npm packages may remain personally owned.

A T2 creation request does not reserve the package name. Its T3/T4 team approval is the first review stage. When the
repository also reviews new packages, the same task then moves to repository moderators and the name remains available
until final approval. This sequencing prevents either permission boundary from bypassing the other.

The same binding is available to Cargo crates, Maven artifacts, and Maven publishing domains. Effective access is the
higher of the explicit package permission and the mapped team role. Team members are never copied into package member
tables. A bound team cannot be deleted until every project and publishing domain has been transferred or removed.
An L4 owner requests transfer in `/account/reviews`; a T3/T4 team manager or system administrator decides it once.

## Limits

Global defaults are stored in `super_teams.create_limit` and `super_teams.join_limit`. The defaults are five created
teams and twenty memberships. Owned teams count toward both values.

The signed-in account reads effective limits and usage through GET /api/super-teams/limits. Managers read or update an
account through GET /api/super-teams/users/{username}/limits and PUT /api/super-teams/users/{username}/limits. An
override of `-1` inherits the global value; zero prevents the corresponding action. Managers configure global defaults
through GET /api/settings/super-teams and PUT /api/settings/super-teams.

## Team lifecycle

GET /api/super-teams returns a bounded, prefix-sorted page. Ordinary accounts see only their memberships; system
administrators see all teams. POST /api/super-teams reserves the prefix and creates the caller as T4. The prefix accepts
2–64 lowercase letters, numbers, hyphens, or underscores, must begin and end with a letter or number, and cannot change.

GET /api/super-teams/{prefix} returns metadata and username-only members. PUT /api/super-teams/{prefix} changes the
display name and description. DELETE /api/super-teams/{prefix} deletes the team and atomically cancels pending
invitations.

## Membership workflow

POST /api/super-teams/{prefix}/members accepts one to twenty usernames and one T1-T4 role. Normal team managers create
seven-day, one-time message-center invitations. System administrators add valid accounts immediately.

PUT /api/super-teams/{prefix}/members/{username} changes a role. DELETE
/api/super-teams/{prefix}/members/{username} removes a member or lets the caller leave. POST
/api/super-teams/invitations/{id}/{decision} accepts `accept` or `reject`; a concurrent or repeated response cannot
apply
the invitation twice.

## API-token boundaries

Global-team routes require `team:manage`; exact restrictions use `global/{prefix}`. Account limit reads use
`account:read`, account override routes use `admin:users`, and global settings use `admin:settings`. A restricted token
cannot list all teams or create a prefix outside its exact target.

Management failures return a stable `X-Renop-Error-Code` and a bounded generic body. Clients should branch on the HTTP
status and registered code rather than displaying response text.
