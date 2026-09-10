# AGENTS.md

RenoP is a Go package repository server with an embedded SPA and a separate static website.
Use this file as a work guide and source index, not a feature catalog.

## Working contract

- Follow the current task's scope, acceptance criteria, and stop/report-only instructions. Continue authorized work;
  ask only when missing information blocks a consequential decision.
- Start with `git status --short --branch` and the relevant diff. Preserve unrelated edits, local data, and secrets.
  Never reset, clean, overwrite, or stage someone else's work.
- For multi-step work, keep a short checklist of requirements, affected layers, verification, and remaining blockers.
  Update it when scope changes or work resumes; do not create planning files for trivial edits.
- Trace the affected flow before editing: entry point -> authorization -> service -> database/storage -> response/UI.
  Find every caller of a changed shared function; inspect actual types, signatures, existing helpers, and nearby tests.
- Prefer deletion or reuse, then stdlib/native features, installed dependencies, and minimal new code.
  Fix the shared root cause. Avoid speculative abstractions, unrelated refactors, and dependency churn.
- Finish each logical change across all affected layers and fix regressions it introduces. Report unrelated findings
  separately. Do not reduce explicitly requested behavior to make a smaller diff.
- Commit/push only within the active task's authorization. Use standard English commit messages; when per-item delivery
  is requested, verify and deliver each item before the next. After pushing, verify HEAD matches the intended remote
  branch.
- Update this file in the same turn when architecture, toolchains, build scripts, workflows, or directories change.
  Replace the affected entry; keep implementation details, API fields, and historical fixes in their owning source/docs.

## Locate the owner

Use `rg --files <directory>` to find files and `rg -n '<symbol>' <directory>` for definitions/callers.
Start with the smallest relevant directory; widen only when references cross its boundary. Batch independent reads,
limit output, and retain useful path/symbol findings instead of repeatedly dumping files or scanning the whole repo.
Skip dependencies, generated bundles, and local storage unless the task concerns them.

Service paths in this table are relative to `internal/service/`; all other paths explicitly start at the repo root.

| Area / symptom                                                         | Start here                                                                                                                                                                                                                                                   |
|------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Startup, shutdown, configuration, shared state                         | `server.go`, `internal/bootstrap/`, `internal/config/`, `internal/core/`                                                                                                                                                                                     |
| HTTP routing, search, middleware, public API                           | `internal/api/`, `internal/middleware/`, relevant service `routes.go`                                                                                                                                                                                        |
| SQL, migrations, transactions, persistence caches                      | `internal/database/`; dialect logic in `clickhouse*.go`                                                                                                                                                                                                      |
| Memory, Redis, Valkey cache backends                                   | `internal/cache/`, `internal/core/cache.go`, `internal/database/cache.go`; configuration in `settings/cache.go`                                                                                                                                              |
| Legal documents and browser consent | `internal/config/legal.go`, `legal/`, `settings/legal.go`; public pages and consent in frontend `js/legal-*.js`, `js/cookie-consent.js` |
| Login, sessions, Passkey, TOTP, OAuth, API tokens, profiles            | `auth/`; second factors in `mfa*.go`; email ownership in `internal/database/account_emails.go`, verification in `email_verification.go`, `github_email.go`; provider flows in `github_*.go`, `oauth_*.go`; OAuth configuration in `internal/config/oauth.go` |
| Registration, retirement, recovery, avatars                            | `auth/`, `internal/database/`, matching `registration*`, `account_retirement*`, `recovery_codes*`, `password_reset*`, `avatar*` files; registration policy in `internal/config/registration.go` and `settings/registration.go`                               |
| Account and IP suspensions                                             | `token/routes.go`, `internal/database/account_ban.go`, `internal/database/account_ip_ban.go`; request enforcement in `internal/middleware/anomaly.go`                                                                                                        |
| Cargo registry and documentation                                       | `cargo/`, `cargodocs/`                                                                                                                                                                                                                                       |
| Maven domains, verification, artifacts                                 | `maven/`                                                                                                                                                                                                                                                     |
| Docker Registry v2, blobs, manifests, mirrors                          | `docker/`                                                                                                                                                                                                                                                    |
| npm metadata, tarballs, versions, dist-tags                            | `npm/`                                                                                                                                                                                                                                                       |
| Files, Disk/S3, quarantine, mirror completion                          | `storage/`, `gpg/`, `packagestore/`; shared mirror hook in `storage/mirror.go`                                                                                                                                                                               |
| Missing/stale index entries, file-vs-directory errors                  | `index/`; HTTP classification in `storage/`                                                                                                                                                                                                                  |
| Upload/configuration races                                             | `repositorygate/`, affected protocol, database transaction                                                                                                                                                                                                   |
| Global teams, ownership, public resources                              | `superteam/`, `ticket/`, `internal/database/super_team_resources.go`                                                                                                                                                                                         |
| Tickets, reports, publication review and notifications                          | `ticket/`, `ticketnotify/`, `internal/database/ticket*.go`, `internal/database/review*.go`                                                                                                                                                                                                   |
| Quota, statistics, global/activity logs, messages, periodic work       | `publicationquota/`, `statistics/`, `audit/`, `message/`, `tasks/`                                                                                                                                                                                           |
| Email transports, templates, durable queue, accounting                 | `internal/mail/`, `mailqueue/`, `internal/database/mail*.go`, `settings/mail.go`; disposable domain data in `internal/mail/data/`, refreshed by `scripts/update-disposable-domains.ps1`                                                                      |
| Outbound networking                                                    | `proxy/`, `outboundproxy/`                                                                                                                                                                                                                                   |
| Updates, services, Caddy                                               | `updater/`, `internal/daemon/`, `internal/caddy/`, `internal/version/`                                                                                                                                                                                       |
| Shared bounds, secret encryption, renames, memory tuning, test cleanup | `internal/utils/` (AES-GCM in `secretcipher/`), `internal/testutil/`                                                                                                                                                                                         |
| SPA and embedded assets                                                | `internal/service/frontend/`; sources in `renop-html/`, embedding in `html.go`                                                                                                                                                                               |
| Shared UI / website / documentation                                    | `packages/renop-ui/`, `web/`, `web/content/docs/`                                                                                                                                                                                                            |
| API/session schemas and generated Go bindings                          | `proto/`, `pkg/pb/`                                                                                                                                                                                                                                          |
| Build, compression, release publishing                                 | `build.ps1`, `scripts/`, `cmd/`, `.github/workflows/`, `.github/scripts/`                                                                                                                                                                                    |

### Frontend reuse map

Paths below are relative to `internal/service/frontend/renop-html/`.
Check `packages/renop-ui/package.json` exports before building another shared control.

| Concern                                                               | Existing owner                                                                                                                                                                                                                                                     |
|-----------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| History, sign-in page, protected routes, HTTP failures, offline state | `js/main.js`, `js/login-route.js`, `js/auth.js`, `css/account-pages.css`, `js/protected-route.js`, `js/api.js`, `js/response-errors.js`, `js/backend-availability.js`                                                                                              |
| Identity, profile photos/links, provider connections, profile cache   | `js/user-profiles.js`, `js/profile.js`, `js/profile-avatar.js`, `js/profile-links.js`, `js/account-language.js`, `js/oauth.js`, `js/components/user-avatar.js`                                                                                                     |
| Account security / recovery / retirement / tokens / administration    | `js/account-security.js`, `js/account-emails.js`, `js/fido-utils.js`, `js/mfa-login.js`, `js/profile-email-verification.js`, `js/account-recovery.js`, `js/password-recovery.js`, `js/login-route.js`, `js/account-retirement.js`, `js/api-tokens.js`, `js/users/` |
| Email accounts, provider presets, billing, delivery                   | `js/settings.js` owns independent pages and in-memory drafts; domain forms in `js/settings/`; email in `js/settings/mail.js`                                                                                                                                                                                            |
| Tickets / reports / messages / administrator composer                           | `js/tickets.js`, `js/ticket-report.js`, `js/ticket-messages.js`, `js/messages.js`, `js/notification-composer.js`                                                                                                                                                                          |
| Teams and quota                                                       | `js/super-teams.js`, `js/super-team-resources.js`, `js/publication-quota.js`                                                                                                                                                                                       |
| Repository/package UI                                                 | `js/browser/`; reuse `repository-view.js`, `package-detail-tabs.js`, `copy-feedback.js`, `user-suggestions.js`; lifecycle in `js/package-deprecation.js`; lock controls in `js/resource-locks.js`                                                                                                           |
| Markdown, clipboard, timestamps, async buttons                        | `js/markdown.js`, `js/clipboard.js`, `js/time.js`, `js/components/button.js`                                                                                                                                                                                       |
| Localization and API error codes                                      | `js/i18n/<locale>/`, `scripts/i18n-catalog.mjs`, `js/*-errors.js`                                                                                                                                                                                                  |
| Shared controls, animation, styling, jQuery runtime                   | `@renop/ui` exports and matching styles in `packages/renop-ui/`                                                                                                                                                                                                    |
| Build and size budgets                                                | `build.mjs`, `rolldown.config.mjs`                                                                                                                                                                                                                                 |

## Preserve these contracts

Read the relevant implementation and tests for exact limits and exceptions before changing these areas.

- **Database:** Support SQLite, MySQL, PostgreSQL/pgx, and native ClickHouse. Use existing transaction/rebinding APIs.
  ClickHouse uses `clickhouse.Open` + EmbeddedRocksDB, never the `database/sql` adapter; preserve journal recovery and
  restart-safe copy/verify/rename schema migrations. Production SQL DELETE statements need a statically provable WHERE.
  Keep SQLite shutdown/WAL cleanup and use `internal/testutil.TempDir` for file-backed database tests.
- **Repository configuration:** `internal/database/repository_settings.go` persists complete snapshots; startup migration
  lives in `internal/bootstrap/repositories.go`. Import legacy `repositories.yaml` only before the initial database
  snapshot, archive it after commit, and never recreate defaults for an existing empty repository set.
- **Identity and authorization:** Ownership/membership uses immutable user IDs; public profiles use `/user/<username>`.
  Enforce live account, repository, package/team, and scoped-token permissions server-side. API tokens intersect current
  owner permissions; cookie-only sessions gate browser-only operations. Basic/password authentication is protocol-only.
  Moderator roles do not imply write or manager authority. Never expose secrets or private profile/team fields publicly.
  OAuth subjects are scoped to the provider configuration authority; never merge accounts by email.
  Primary and provider email addresses share immutable account ownership; binding and email reservations commit
  together. Unverified provider addresses require existing ownership or email proof; retained aliases survive unlinking.
  Preserve PKCE, OIDC signature/issuer/audience/nonce verification, browser-bound state, and mandatory email proof
  during provider registration.
  Browser session issuance enforces live second-factor policy and credential snapshots. Secondary Passkeys cannot count
  as primary login methods; MFA accounts use API tokens for package clients. Startup persists the private authenticator
  encryption key before serving requests; preserve it across configuration updates.
- **Account lifecycle:** Preserve alternate-login and atomic recovery-consumption invariants, hashed credentials,
  immediate revocation, and targeted cache invalidation. All login methods honor bans and retirement.
  Administrators and moderators must lose those roles before suspension; suspended accounts cannot gain those roles.
  Optional IP restrictions share the account ban's lifetime, use recorded login addresses, and commit with session
  revocation. Preserve trusted-proxy extraction, local cache invalidation, and overlapping bans on shared addresses.
  Registration is explicitly enabled; confirmation, credentials, email, provider identity, and persistent IP accounting
  commit together. Pending provider registrations confer no account privileges. OAuth callbacks require the initiating
  browser cookie as well as a single-use server state.
  Retirement rechecks protected roles, ownership, and pending reviews; keeps permanent tombstones, reserves email for
  14 days, retains audit activity for 30 days, and rejects stale writes or delayed audit resurrection.
- **Teams and packages:** Preserve live T1-T4/L0-L4 permission mapping, the last owner, membership limits, and public
  visibility filters. Scoped npm and namespaced Docker require the matching global-team prefix. Reserve npm/Docker
  resources explicitly, check upstream conflicts, and keep mirrored packages pull-only. Permanent package deprecation
  blocks every mutation while retaining downloads; pending transfers/reviews prevent deprecation.
- **Resource locks:** Shared records live in `internal/database/resource_lock.go`; protocol enforcement lives in
  `internal/service/cargo/locks.go`, `internal/service/npm/locks.go`, `internal/service/docker/locks.go`, and
  `internal/service/maven/locks.go`. Maven XML projections live in `internal/service/maven/metadata_locks.go`;
  preserve SNAPSHOT and companion-path coverage in `internal/database/maven_locks.go`.
  Docker index references are captured in `internal/database/docker_locks.go`; preserve source-specific inheritance
  and shared-blob protection. Cross-repository mounts acquire ordered gates in `repositorygate/`.
  Shared mirror authorization is wired in `internal/service/storage/mirror.go`.
  Global-team lock routes live in `internal/service/superteam/locks.go`; bindings inherit restrictions through
  `internal/database/resource_lock.go`. Maven domain/path inheritance lives in `internal/database/maven_domain_locks.go`.
  Global publishing-domain lock routes live in `internal/service/maven/locks.go`; repository changes also inspect
  uncatalogued domain namespaces through `EnsureRepositoryMutable`.
  Preserve retained memberships, independent verified child domains, and current-request moderator scopes.
  Store bounded custom reasons in `resource_locks.reason_text` and render them as plain text.
  Keep manual and system locks independent. Read locks freeze writes, restrict metadata to live staff/members, and deny
  files to everyone. Include index/search/profile visibility, cached files, mirror refreshes, and repository changes.
- **Maven lifecycle:** Domains are global across repositories. Closure blocks mutations, preserves downloads, and holds
  the name for 31 days; a later claimant must verify ownership and obtain administrator approval before publication.
  Security monitoring in `maven/domain_health.go` uses RDAP or pinned provider account IDs. Preserve explicit redemption,
  configurable calendar reservations (two years by default), and moderator-approved restoration of reclaimed artifacts;
  persistence lives in `internal/database/maven_health.go` and `maven_restore.go`.
  Preserve classic/catalog layouts and Maven/files migration without moving stored objects.
- **Review:** Decisions and metadata changes commit once, atomically, with live authority rechecked. Pending
  publications
  stay out of public protocol responses and index paths, including after restart/GPG completion. Creation uses
  `@create`;
  T2 requests pass team approval before any repository stage. Virtual Docker manifests are not filesystem objects;
  rejection must not delete shared blobs. Block repository reconfiguration/migration/deletion while publication reviews
  are pending. Notify after durable decisions, deduplicate recipients, and expose handling staff identity only to authorized staff inside tickets, never to the requester.
  Reports hide reporter identity from targets; assignment and escalation commit atomically with live authority.
- **Storage and concurrency:** Reuse `packagestore/` for bounded staging, validation, atomic Disk/S3 commit, and
  rollback;
  use `repositorygate/` around mutations that race configuration, review, or retirement. Stream large bodies/archives,
  validate actual bytes as well as advertised sizes, verify hashes, bound extraction paths and work queues.
  Replace files atomically; failed staging/rename must preserve the installed destination and recoverable source.
- **Visibility and resource bounds:** A normalized index path is a file or directory, never both; known artifacts never
  receive the SPA shell. Bound cache size/lifetime, request and response reads, pagination, batches, and background
  work.
  Invalidate affected caches after successful mutations and guard against stale in-flight fills. Avoid N+1 queries,
  redundant hot-path copies, and whole-object buffering.
  External cache values are encrypted with process-private keys; keep bounded local invalidation indexes so cache
  outages cannot undo credential revocation. Backend changes require a restart; never use Redis as authoritative
  storage.
- **Quota and events:** Reserve/commit/release quota transactionally; team-owned resources charge only the team and
  mirrors are exempt. Keep download-count exclusions and pending-plus-persisted resets in `statistics/`.
  Use `tasks/` for coalescible periodic work; preserve dedicated serial workers where event order matters.
  `audit/` captures process/HTTP diagnostics, filters activity and global logs, and drains its shared serial writer on
  shutdown.
  Preserve hidden-operator filtering, credential redaction, and separate activity/system retention budgets.
- **Email:** `mailqueue/` owns the durable serial worker, rate limits, credit reservations, and status checks.
  Preserve encrypted queue payloads, persistent OAuth rotation, bounded history, and private status capabilities.
  An interrupted submission has an unknown outcome; never resend it automatically or treat acceptance as delivery.
- **Legal documents:** `config.yaml` owns the three Markdown documents; no local policy file or external legal URL is read.
  Keep bounded safe rendering and public access with expired credentials. Account entry requires the current privacy/terms
  revision; optional browser services require explicit category consent, with preferences available from the footer.
- **Frontend:** Reuse the shared UI, jQuery runtime, error, identity, clipboard, time, and animation helpers.
  Embedded static assets stream from `embed.FS`; cache representation metadata rather than duplicate payload bytes.
  Shared custom selects create menus and global listeners only while open; preserve detach/remount and keyboard behavior.
  Keep streaming/observers/native APIs where appropriate. Preserve keyboard/focus behavior, responsive layouts,
  viewport-bounded dialogs, and loading/empty/error states. A valid authenticated 403 must not log out the user.
  Render untrusted Markdown through the inert allowlist; never show raw backend errors or runtime exceptions.
- **API encoding:** `internal/utils/protohttp/` negotiates protobuf and ProtoJSON for schema-backed control APIs.
  Preserve the legacy protobuf default, Content-Type request selection, Accept response selection, request bounds,
  and Vary headers. Native package protocols and binary uploads retain their own wire formats.
- **Localization/docs:** Add stable errors and audit actions to every supported locale. English is canonical;
  `internal/locale/` matches account and request languages; private preferences live in `user_profiles.locale`.
  `internal/mail/template_locales.go` covers every frontend language. Mail captures the recipient language when queued;
  browser language synchronization binds pending changes to immutable user IDs and preserves credential revisions.
  preserve keys/placeholders, lazy locale loading, and bundle budgets. Website translations must retain canonical
  files, heading outlines, examples, endpoints, and links; see `web/test/docs-parity.test.mjs`.
- **Release:** Frontend sidecars must not be recompressed; serve them with correct negotiation, ETags, and Vary.
  Update payloads contain only raw `.br` executables and `manifest.json`; release docs attach separately to GitHub.
  Preserve SHA-256 checks, legacy ZIP decoding, current/previous commit ordering, and bounded nightly retention.
  Nightly metadata is rebuilt by `.github/scripts/nightly-info.ps1`; publishing requires PowerShell 7.5 or later
  to preserve JSON date strings. Cleanup follows the sorted obsolete entries after successful publication;
  preserve retained trees, skip missing directories without consuming the deletion budget, and surface HTTP failures.
  The custom linker receives `-o2` through `-ldflags`; do not pass it as a `go build` flag.
  Actions share the workflow-level `renop-actions` FIFO group with `queue: max`; retain serial job dependencies.
  Compile/compression pools remain independently bounded; see `scripts/build-target.ps1` and
  `scripts/compress-target.ps1`.

## Implementation standards

- Keep comments, docstrings, and engineering documentation in English; translated product content uses its locale.
  Use standard Go doc comments for exported declarations and concise JSDoc for handwritten JS/TS functions.
  Comment non-obvious intent, safety constraints, or tradeoffs; do not narrate statements.
- Preserve validation, error propagation, cleanup, rollback, and accessibility. Do not silence failures, return dummy
  success, weaken assertions, or bypass security checks to make validation pass.
- Modify source, then regenerate affected outputs. Do not hand-edit generated protobuf, locale catalogs, or bundles.
  Inspect generated diffs and preserve license headers. Update lockfiles/notices when dependencies actually change.
- Check affected API/schema compatibility, all callers, frontend wiring, permissions, i18n, migration/rollback,
  cache invalidation, and docs as part of the change; irrelevant layers require no work.

## Verification by impact

Choose checks from the changed behavior and its callers. The table is a selection guide, not a mandatory full checklist.
Run existing checks first. Non-trivial behavior needs a runnable check of its contract; reuse sufficient coverage.
Add or extend the smallest regression test only when existing checks would miss the changed behavior or defect.
Use the current Go/Node test setup; avoid duplicate or implementation-mirroring tests, broad fixtures, new frameworks,
and test files for prose/trivial edits.

| Change                                                      | Required relevant evidence                                                                                                             |
|-------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| Prose-only engineering docs/comments (no directives)        | Review accuracy, paths/commands, and `git diff --check`; no application build/test                                                     |
| Website documentation                                       | `pnpm run test:web`; add `pnpm run build:web` when rendering/build behavior is affected                                                |
| Local Go behavior                                           | Affected package tests and caller tests; targeted regression/boundary cases for changed behavior                                       |
| Shared backend contracts, Go dependencies, broad Go changes | `go test -count=1 -p 1 ./...` and `go vet ./...`; local build when startup/toolchain/embedding is affected                             |
| Shared SQL/schema/transaction semantics                     | Database + affected service tests; isolated contract runs on all four drivers; report unavailable drivers explicitly                   |
| SPA JS/CSS/HTML                                             | Relevant existing frontend tests for behavior, `pnpm run build:frontend`; inspect changed UI when browser access is available          |
| Shared UI                                                   | Relevant tests in both consumers; `pnpm run build:ui-consumers`                                                                        |
| Locale keys/errors/audit actions                            | `pnpm run check:i18n` and relevant coverage tests; frontend build already includes i18n validation                                     |
| Protobuf/API contract                                       | Regenerate affected Go/JS bindings, test affected producers/consumers, verify compatibility; local build for integration               |
| Build/workflow/packaging                                    | Script syntax and affected build path; validate release payload for packaging changes; full target matrix only when affected/requested |
| Security/concurrency/performance                            | Reproduce the issue; check denial/boundary/rollback cases; race detector or focused benchmark only when that risk is changed           |

- Start focused, then expand for shared contracts or unresolved risk. A build proves compilation, not behavior.
  Do not rerun overlapping full suites after successful final checks unless code, failures, or relevant evidence
  changes.
- Validate the final edited state; earlier results do not cover later relevant edits. Track passed, failed, and unrun
  checks. If tooling/data/browser access blocks a required check, report the gap instead of claiming full verification.
- Prefer serial Go package tests (`-p 1`) on Windows; avoid competing builds over shared generated files/caches.
  Diagnose locks, permissions, network, and toolchain failures before attributing them to code; never weaken tests.
- `renop-dbtest` is destructive: use a fresh, empty, disposable DSN per run with `-confirm-isolated`; its temporary
  mail encryption keys cannot decode queue data from earlier runs. Never use local/live
  application data or print credentials. Benchmark only when needed to validate a performance claim.

## Commands and toolchain

Run from the repository root using PowerShell 7. Toolchain sources of truth: `go.mod` and
`.github/actions/setup-go-runtime/` (404Setup/go), `package.json` (Node minimum and pnpm pin),
`.github/workflows/build.yml`
(Node/protoc).
Install missing prerequisites; do not silently replace the custom Go runtime or upgrade pins to bypass a failure.

| Task                                                  | Command                                                                            |
|-------------------------------------------------------|------------------------------------------------------------------------------------|
| Restore JS dependencies when missing/lockfile changed | `pnpm install --frozen-lockfile`                                                   |
| Focused Go tests (substitute package and test name)   | `go test -count=1 -p 1 ./internal/service/<module> -run '<TestName>'`              |
| Frontend / website tests                              | `pnpm run test:frontend` / `pnpm run test:web`                                     |
| One frontend test (substitute file)                   | `node --test internal/service/frontend/renop-html/test/<name>.test.mjs`            |
| Frontend / website build                              | `pnpm run build:frontend` / `pnpm run build:web`                                   |
| Both UI consumers                                     | `pnpm run build:ui-consumers`                                                      |
| Go protobuf generation                                | `go generate ./pkg/pb`                                                             |
| Browser protobuf generation                           | `pnpm --filter renop-html run proto`                                               |
| Local integrated build, unpackaged                    | `pwsh ./build.ps1 c nb`                                                            |
| Current-platform packaged / full release matrix       | `pwsh ./build.ps1 c` / `pwsh ./build.ps1`                                          |
| Release payload check                                 | `pwsh ./.github/scripts/test-release-payload.ps1 -DistDir ./dist`                  |
| Disposable database contract                          | `go run ./cmd/renop-dbtest -driver <driver> -dsn <isolated-dsn> -confirm-isolated` |

The optional browser contract in `test/custom-select-browser.test.mjs` uses an already running browser.
Set `RENOP_TEST_BROWSER_CDP` to its debugging origin; `RENOP_TEST_BROWSER_URL` and
`RENOP_TEST_SELECT_SELECTOR` select the loaded page and control. It changes and restores one draft selection.

`build.ps1` generates both Go schemas and builds the frontend (i18n, JS protobuf, bundles, precompression).
Full frontend builds use PowerShell 7 to prepare the ignored `internal/mail/data/` before Go compilation, including CI
builds.
`scripts/update-disposable-domains.ps1` pins source revisions and checksums, reuses verified local data, and fails on
download or integrity errors. Keep generated mail data out of Git; update source pins and notices when refreshing it.
`go generate ./...` also installs/builds the frontend; do not stack it with an equivalent completed frontend build.
Generation requires `protoc`/`protoc-gen-go`; frontend precompression also requires Go.
Builds regenerate assets and may replace `dist/`; inspect the final diff and output, not just the exit code.

## Before delivery

- Recheck each acceptance criterion and the final diff; resolve TODOs introduced by this task or report a concrete
  blocker.
- Confirm coupled code, permissions, i18n, docs, and generated outputs are consistent; run `git diff --check`.
- Remove only temporary artifacts created by this task; preserve unrelated changes, ignored local data, and secrets.
- Report the result, actual verification, and material remaining gaps concisely; include commit/push status only when
  relevant.
  Never call a plan, partial implementation, failed check, or unpushed change a completed delivery when more was
  requested.
