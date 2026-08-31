---
title: Production Deployment Checklist
order: 4
category: Deployment
description: Security, persistence, proxy, repository, validation, monitoring, and rollback checks before go-live
---

# Production Deployment Checklist

Use this checklist after the first successful startup and before allowing package clients or untrusted networks to
reach the service. A passing health probe is necessary, but it does not prove that authentication, the database,
storage, mirrors, or publication policy work end to end.

## Define the service boundary

Document the public hostname, listener, reverse proxy, database, storage backends, and repository owners. Treat RenoP
as one coordinated service. An external database or S3-compatible backend replaces a local state component; it does
not, by itself, provide safe active-active coordination.

- Assign one operational owner and one security contact.
- Record the RenoP version, configuration paths, service account, working directory, and update channel.
- Decide which repositories are public, hidden, or private before publishing client configuration.
- Keep the management interface and package endpoints on the same canonical HTTPS origin unless the complete proxy and
  cookie behavior has been tested on every published origin.

## Secure bootstrap and account recovery

Set `RENOP_DEFAULT_ADMIN_PASSWORD` only for the first creation of the `admin` account. If RenoP generated the password,
retrieve it from the first-start log and replace it immediately.

- Create named administrator accounts instead of sharing `admin` for routine work.
- Register a Passkey or configure another tested login method before disabling password login.
- Generate recovery codes, store them offline, and verify that the account email is correct.
- Issue separate, expiring API tokens for CI jobs. Grant only the scopes and repository targets each job needs.
- Store database, S3, OAuth, SMTP, signing, and proxy credentials in a secret manager or protected service environment.

## Publish through HTTPS

Bind RenoP to a loopback or private address when a reverse proxy terminates TLS. Configure the public host and only the
proxy addresses that are allowed to supply client IP headers.

```yaml
server:
  host: "127.0.0.1"
  port: 3000
  domains:
    - "packages.example.com"
  trusted_proxies:
    - "127.0.0.1"
  cdn_ip_header: "X-Forwarded-For"
```

The proxy must preserve `Host`, the original scheme, and the client-address chain. Disable request buffering for large
uploads, remove an accidental body-size ceiling, and set read/write timeouts long enough for image layers and large
artifacts. Do not trust forwarding headers from arbitrary clients. See [Reverse Proxy Setup](./reverse-proxy.md).

## Protect the database and artifact storage

Choose a database appropriate for the deployment and test it with a real authenticated write. For SQLite, place the
database on durable local storage and ensure the service account owns the file and its directory. For an external
database, require encrypted transport where supported and restrict network access to RenoP.

For every repository, confirm the selected local or S3-compatible backend, bucket or directory, prefix, credentials,
and download mode. A presigned redirect must be reachable and trusted by clients; proxy streaming keeps the bucket
private but sends artifact traffic through RenoP.

Prepare a backup procedure that captures configuration, repository definitions, the database, and non-rebuildable
artifact data as one recoverable set. Then perform a restore rehearsal. See
[Backup, Restore & Migration](./backup-and-recovery.md).

## Define repository and publication policy

- Select the correct format for every repository; client protocols are not interchangeable.
- Review visibility, read and publish permissions, team membership, namespace ownership, quotas, and review policy.
- Configure mirrors deliberately. Set timeouts, cache lifetimes, negative caching, and allowlists appropriate to the
  upstream instead of treating a public registry as implicitly trusted.
- Verify Maven publishing domains, reserve npm packages and Docker image names where required, and confirm Cargo name
  availability before announcing a publication path.
- Decide who can approve publication reviews and who can respond to ownership-transfer tasks.

## Validate with native clients

Test the exact hostname, credentials, repository name, and proxy path that users will receive. Include at least one read
and one authorized write for each enabled format. Where lifecycle permissions are enabled, also test delete, yank,
archive, or tag changes in a disposable package.

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

The expected response body is `"UP"`.

Also verify that an anonymous request cannot read a private repository, an under-scoped token is denied, a hidden
repository is absent from discovery, and an oversized or disallowed publication fails without leaving visible partial
state.

## Establish operations and monitoring

- Monitor process availability, storage capacity, database health, certificate expiry, upstream latency, and repeated
  authentication or publication failures.
- Retain service logs outside the application working directory and protect them as potentially sensitive data.
- Review the audit log and in-app messages regularly; neither is a substitute for external alerting.
- Test the selected stable or nightly update workflow in a non-production deployment before enabling automation.
- Set a maintenance window and document who can revoke sessions, tokens, or compromised package ownership.

## Go-live checklist

- [ ] Canonical HTTPS hostname resolves from every required client network.
- [ ] Proxy body limits, buffering, timeouts, and forwarded headers were tested with a large upload.
- [ ] Administrator recovery methods and offline recovery codes are available.
- [ ] CI uses scoped, expiring tokens rather than a personal password.
- [ ] Database and every artifact backend passed a write/read/delete test.
- [ ] Repository visibility, ownership, quotas, mirrors, and review policy were reviewed.
- [ ] A private repository remains private through direct and proxied URLs.
- [ ] Backups exist in a separate failure domain and a restore rehearsal succeeded.
- [ ] Capacity and certificate alerts have named recipients.
- [ ] The current binary, configuration, and rollback procedure are recorded.

## Prepare rollback before changing production

Keep the previously working binary, configuration files, and database/storage backup until the new version has passed
client validation. Restore the previous application version together with a mutually compatible data snapshot; do not
assume that a database changed by a newer release can always be used by an older binary. Record the reason, timestamps,
and affected repositories for every rollback.

For protocol-specific validation, continue with the [Maven](../guides/maven-client.md),
[Cargo](../guides/cargo-registry.md), [npm](../guides/npm-registry.md), and
[Docker/OCI](../guides/docker-registry.md) guides.
