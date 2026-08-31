---
title: Backup, Restore & Migration
order: 5
category: Deployment
description: Consistent backups, restore rehearsals, backend migration, and disaster-recovery validation
---

# Backup, Restore & Migration

A RenoP backup is complete only when configuration, repository policy, database state, and non-rebuildable artifact data
can be restored together. Copying `index.json` or an S3 bucket alone is not enough.

## Classify the data

| Data                        | Typical location                            | Recovery role                                                                 |
|:----------------------------|:--------------------------------------------|:------------------------------------------------------------------------------|
| Main configuration          | `config.yaml` or `RENOP_CONFIG`             | Listener, database, proxy, security, previews, updater                        |
| Repository definitions      | `repositories.yaml` or `RENOP_REPOSITORIES` | Format, visibility, mirrors, storage backend, policy                          |
| Database                    | `renop.db` or external DSN                  | Accounts, permissions, sessions, tokens, teams, reviews, audit, messages      |
| Local artifact data         | `storage_path`                              | Published packages, uploads, cached upstream content                          |
| S3-compatible artifact data | Bucket and per-repository prefix            | Published packages and cached content for S3-backed repositories              |
| File index                  | `index.json` or `RENOP_INDEX`               | Performance snapshot; useful to keep, but rebuildable from authoritative data |
| TLS and integration secrets | Proxy or secret manager                     | Required to restore the same public service and integrations                  |

Generated website output, downloaded frontend dependencies, and build caches can be recreated and should not be the
only copy of any operational secret.

## Choose a consistency point

The safest general procedure is a cold backup: block new traffic, stop RenoP cleanly, snapshot the database and artifact
backends, copy configuration, and then restart the service. This avoids a database record referring to an object that
was captured at a different point in time.

When downtime is unacceptable, use transactionally consistent database backups and storage snapshots with a documented
common recovery point. A live copy of only the SQLite main file is unsafe while write-ahead-log files may still contain
committed data. Provider snapshots are not consistent merely because they were started at similar times.

## Back up a local SQLite deployment

Stop RenoP through the same service manager that starts it. After the process has exited, copy the closed database,
configuration, repository file, index snapshot, and local storage tree.

```bash
install -d /backup/renop
cp config.yaml repositories.yaml renop.db index.json /backup/renop/
rsync -a storage/ /backup/renop/storage/
```

Use the paths configured through `RENOP_CONFIG`, `RENOP_REPOSITORIES`, `RENOP_INDEX`, the database DSN, and
`storage_path`; the example names are defaults. Preserve file ownership, permissions, extended attributes where
required, and sufficient free space for temporary upload files.

## Back up external databases

Use the database vendor's supported logical dump, physical backup, or managed snapshot mechanism. Include all RenoP
tables and migration metadata. Encrypt backup traffic and files, retain the server version and database-engine version,
and verify the backup with the vendor's restore tooling.

For MySQL or PostgreSQL, prefer a consistent snapshot that includes all tables in one transaction or recovery point. For
ClickHouse, follow the operational requirements of the configured deployment and retain any data needed by RenoP's
transaction journal. Do not reconstruct account or team state from the web UI after losing the database.

## Back up local and S3-compatible artifacts

For local storage, copy the complete configured root. Do not select files by extension: metadata, manifests, package
indexes, signatures, and upload state can be as important as the main archive.

For S3-compatible storage:

- Protect every bucket and `key_prefix` used by a repository.
- Enable versioning or replication when supported and test object recovery, not only object listing.
- Keep backup credentials separate from the credentials used by RenoP.
- Preserve object metadata and verify that lifecycle rules cannot remove the only retained copy too early.
- Keep the bucket private unless a deliberate presigned-download design requires otherwise.

Mirror caches can usually be repopulated, but locally published artifacts may be irreplaceable. Apply different
retention rules only after you can distinguish them reliably.

## Restore in a controlled environment

Restore to an isolated host or network first. Use the same RenoP version that created the backup, confirm that the
restored service works, and then perform an upgrade separately if required.

1. Restore `config.yaml`, `repositories.yaml`, certificates, and integration secrets with restrictive permissions.
2. Restore the database and verify that its configured hostname, credentials, and TLS settings are valid.
3. Restore local storage or reconnect the exact S3 bucket and prefix.
4. Restore `index.json` if available; otherwise allow RenoP to rebuild indexes from authoritative storage.
5. Start RenoP without public traffic and inspect startup errors.
6. Sign in, list repositories, and test representative package reads.
7. Publish and remove a disposable package using a narrowly scoped token.
8. Re-enable traffic only after authorization, mirrors, reviews, quotas, previews, and audit recording are verified.

After a security incident, restoring valid sessions and tokens may be undesirable. Revoke sessions, rotate API tokens,
and replace database, storage, OAuth, SMTP, proxy, and signing credentials according to the incident scope.

## Migrate a repository backend

Use RenoP's repository-management migration path so that repository operations are serialized with the backend change.
Do not edit physical directories, copy live objects behind RenoP, or switch the configuration before the copy has been
verified.

Before migration, record package and version counts, total bytes, repository policy, source and destination settings,
and available free capacity. After migration, compare listings and representative hashes, test native-client reads and
writes, and retain the source as a read-only rollback copy until the acceptance window ends.

## Run recovery drills

Define a recovery point objective and recovery time objective for the complete service, not only the database. At least
periodically, restore the newest backup into an empty environment and record:

- backup start and completion timestamps;
- the versions of RenoP, the database, and storage services;
- restore duration and manual steps;
- package read/write verification results for every enabled format;
- missing objects, permission errors, stale DNS or certificates, and follow-up actions.

A backup that has never been restored is an untested assumption. Link the final runbook from the
[Production Deployment Checklist](./production-checklist.md) and keep an offline copy available during an outage.
