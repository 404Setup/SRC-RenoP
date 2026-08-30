# AGENTS.md

> **CRITICAL MANDATE**:
> Whenever you modify architecture, toolchains, build scripts, workflows, or directory structures, **update this
`AGENTS.md` in the same turn**.

---

## 1. Project Architecture & Core Modules

**RenoP** is a high-performance self-hosted package repository server with a **Go** backend and an embedded
**Node.js/pnpm** frontend.

- **`server.go`**: Application entry point and server lifecycle.
- **`cmd/renop-brotli/`**: Streaming Go CLI installed automatically by `build.ps1` to encode each release executable
  as a raw RFC 7932 Brotli stream with `github.com/molecule-man/go-brrr`.
- **`cmd/renop-dbtest/`**: Standalone destructive-on-isolated-data driver contract CLI. It requires
  `-confirm-isolated` and exercises account/session persistence, rollback, message deduplication, Cargo/Docker/Maven/npm
  catalogs, global-team invitation/role mutations, cross-engine team bindings, and download statistics through the same
  database API used by the server. Its review phase also verifies repository-moderator listing, bounded publication
  files, single-decision completion, and hidden-path release across every available driver.
- **`scripts/build-target.ps1` & `scripts/compress-target.ps1`**: Isolated release workers coordinated by `build.ps1`.
  Up to four compilations run independently from up to eight Brotli packaging tasks; a completed compilation releases
  its build slot immediately and queues compression without delaying the next architecture. The parent preserves
  deterministic manifest order and aggregates failures from both pools. The `dist/` update payload is restricted to
  raw `.br` packages plus `manifest.json`; `.github/scripts/test-release-payload.ps1` enforces that boundary before
  the update API is called. Nightly publishing retains the latest three package trees with downloadable target metadata
  while preserving up to 100 lightweight release-history records; older package trees are deleted in bounded batches.
  License, README, and third-party notices are attached to GitHub releases directly from the checkout and are never
  uploaded to the update API.
- **`internal/database/`**: Pluggable multi-dialect DB (SQLite, MySQL, PostgreSQL via `jackc/pgx/v5`, and native
  ClickHouse via `clickhouse.Open` from `clickhouse-go/v2`). ClickHouse uses mutable `EmbeddedRocksDB` tables with
  materialized collision-free composite keys, synchronous mutations, portable scan conversion, and a serialized
  row-level snapshot journal that restores interrupted multi-statement transactions at startup; it never uses
  `clickhouse.OpenDB` or the `database/sql` compatibility API. Because `EmbeddedRocksDB` cannot add columns in place,
  startup schema additions use a restart-recoverable server-side `INSERT SELECT` copy, row-count verification, atomic
  table rename, and removable backup instead of buffering rows in RenoP. Connection handling, transaction recovery,
  portable SQL
  parsing, scan conversion, schema declarations, and dialect translation remain isolated in focused `clickhouse*.go`
  modules. SQLite shutdown checkpoints and exits WAL mode before closing pooled connections so Windows can release
  sidecar files deterministically. Bounded sharded read-through caches use randomized zero-allocation key hashing and coalesce concurrent
  token, session, and immutable-user lookup misses;
  nickname-first profile batches query only uncached accounts, and commit-time invalidation prevents stale rename or
  creation results. A repository-wide AST regression test rejects production Go SQL containing any `DELETE FROM`
  statement that cannot statically demonstrate a `WHERE` clause. Includes
  zero-alloc SQL parameter rebinding (`RebindPostgres`), unified transaction wrappers, schema migrations, public user
  profiles, immutable user identities for package ownership, private normalized login emails, serialized login-method
  invariants, masked account-token/profile mutations, irreversible one-time recovery-code verifiers, and hashed,
  expiring fine-grained API credentials. Legacy plaintext upload tokens migrate transactionally to scoped hashes;
  durable GitHub identity/principal snapshots and username-change throttling remain bound to immutable user IDs.
  npm package reservations, immutable versions, dist-tags, L0-L4 teams, and invitations use the same immutable
  identities across every supported SQL dialect. Catalog reads and writes derive a usable latest published version
  when the optional `latest` dist-tag is absent, including automatic repair of older empty summary rows.
  Engine-independent global teams reserve an immutable prefix, store T1-T4 memberships and invitations exclusively by
  immutable user ID, preserve creator display after account deletion, and enforce global or per-account creation and
  membership limits on SQLite, PostgreSQL, MySQL, and native ClickHouse. Cargo crates, Docker images, npm packages,
  Maven artifacts, and Maven publishing domains use one optional indexed `super_team_prefix`; effective authorization
  takes the higher of an explicit package permission and the live T1-T4 mapping without copying team members. Durable
  `review_tasks` preserve immutable request identities, bounded filters, source/target bindings, and a pending-state
  compare-and-set so ownership transfers and their decisions apply atomically across every database driver.
  `review_task_files` attaches at most 256 repository-relative files to each moderated publication without storing
  package bytes in the database; active publication keys merge Maven companions into one version task, enforce an
  upload-settling interval, and preserve rejected or approved decision history. Shared task paging and transfer
  decisions remain in `review.go`, while publication keys, file ownership, and hidden-path queries are isolated in
  `review_publication.go`. Bounded `review_task_payloads` temporarily retain protocol metadata that must be committed
  atomically at approval; payloads are deleted in the same transaction as the final task decision.
  Publication quota overrides, current-period usage, and expiring reservations are keyed by immutable account IDs or
  global-team prefixes. Short serialized mutations prevent parallel publications from exceeding file, byte, or
  publication limits on SQLite, PostgreSQL, MySQL, and native ClickHouse.
- **`internal/service/auth/`**: Password, FIDO/Passkey, session, profile, and GitHub OAuth workflows. GitHub OAuth
  separates bounded single-use route state, constrained provider HTTP access, and collision-safe account linking into
  `github_routes.go`, `github_client.go`, and `github_account.go`; access tokens are never persisted. Account recovery
  uses twelve 160-bit codes, Argon2id verifiers, four-code atomic consumption, and session revocation; password login
  may be disabled only while a GitHub identity or Passkey remains available. API tokens use one-time 256-bit secrets,
  optional expiration, current-account-permission intersection, and immediate revocation. Capabilities separately
  gate repository reads/publication/deletion, package creation/metadata/lifecycle, team administration, and Maven-domain
  reading/creation/verification/deletion. Each target-aware scope can additionally carry bounded exact repository,
  package, team, or domain restrictions in the backward-compatible authorization JSON; legacy broad package/domain
  scopes remain authentication-only compatibility. Team targets also accept bounded `global/<prefix>` restrictions.
  Token secrets are owner-managed from a browser session; administrators cannot mint credentials for another user.
  Browser session secrets are cookie-only, while Basic/password credentials are restricted to package protocols.
  Repository-scoped `canmoderate:<name>` and global `canmoderate:*` roles grant private review visibility without
  repository writes or system-manager authority; `manager` remains the only global configuration bypass.
  Authentication-result invalidation is scoped to the changed account or revoked API token so unrelated hot entries
  remain available; validity-changing operations also remove bounded negative credential results.
- **`internal/service/cargo/` & `internal/service/cargodocs/`**: Sparse Cargo registry implementation, crate lifecycle,
  authoritative upstream name-conflict checks, mirrored-crate provenance, upstream proxying, and sandboxed documentation
  extraction/viewer (`/cargodoc/...`). Local publication streams a bounded Markdown README selected by the validated
  `Cargo.toml` declaration into package metadata without loading the crate archive into memory. Optional new-package or
  every-version publication review commits and hides the crate archive first, then atomically writes the sparse index
  and catalog metadata before approval exposes it; rejection removes the hidden archive and mirrors bypass review.
- **`internal/service/maven/`**: Process-wide Maven domain registry with DNS/GitHub/GitLab ownership verification,
  global L0-L4 domain teams shared by every Maven repository, invitation workflows, catalog/version management, and
  automatic migration of repository-scoped legacy domains. Upstream mirror discovery persists unverified global
  domains so administrators can filter, inspect, and explicitly approve them. Maven and Cargo mirror downloads are
  cataloged through
  the format-aware proxy completion hook in `internal/service/storage/mirror.go` without buffering artifact bodies.
  Maven repositories support modern domain-catalog and classic file-tree layouts while enforcing the same verified
  Maven publication paths in both layouts. Administrators can migrate Maven repositories to the unstructured files
  engine and back without moving stored objects; returning to Maven streams the existing Disk/S3 index into a rebuilt
  catalog and restores the prior Maven layout and publication policy. Artifact detail responses summarize bounded
  primary-file, checksum, and signature metadata from the in-memory index and stream-parse the latest POM up to 2 MiB;
  project collections are capped before they reach the frontend. Artifact teams can maintain a separate bounded
  package-level Markdown README without replacing the short catalog/POM description.
- **`internal/service/docker/`**: OCI & Docker Registry v2 specification implementation (`/v2/...`), token-based
  Bearer authentication, explicitly reserved images, L0-L4 image teams, per-image private visibility, image-scoped
  blob references, chunked uploads, authorized cross-repository mounting, upstream mirror proxying, and catalog
  management. Client pushes cannot create images implicitly; administrators reserve public or private images through
  the frontend first. Local reservations are unique and cannot claim names exposed by an enabled upstream mirror;
  mirror-discovered images remain permanently pull-only. Image README content is editable by package managers and
  bounded to 512 KiB at both the HTTP and database boundaries. Both review policies hold explicit image creation
  without reserving its name; `new_packages` stops after creation approval, while `every_version` also keeps each exact
  manifest in a bounded virtual payload. Approval rechecks the publisher and referenced blobs, then atomically records
  manifest metadata, blob links, the tag, and the review decision. Existing digest files are never hidden by another
  pending tag, rejection leaves shared blobs untouched, and mirror imports bypass review. Local names
  containing `/` require a matching global team prefix. T3/T4 members reserve directly unless repository review is
  enabled; T2 members enter the ordered team-approval workflow. Unprefixed images may remain personally owned.
- **`internal/service/npm/`**: npm-compatible per-repository registry with explicitly reserved public or scoped-private
  packages, immutable semantic versions, validated streaming tarball publication, dist-tags, deprecation/unpublish
  workflows, L0-L4 package teams, upstream packument/tarball mirrors, and full/abbreviated metadata negotiation.
  URL-encoded scoped metadata routes are decoded by the npm protocol before shared file-path sanitization. Mirrored
  packages remain pull-only, while local publication requires both repository and package permission. Scoped local
  packages require a matching global team prefix. T3/T4 members reserve directly unless repository review is enabled;
  T2 members enter the ordered team-approval workflow. Unscoped packages may remain personally owned. Both review policies hold explicit package creation without reserving its name;
  `new_packages` stops after creation approval, while `every_version` also hides each committed tarball and bounded
  manifest/dist-tag payload until a repository moderator approves the same immutable publication transaction. Creation
  and publication decisions are atomic, and decision failure restores the exact prior package summary and revision.
- **`internal/service/proxy/` & `internal/service/outboundproxy/`**: Outbound HTTP/HTTPS/SOCKS5 proxy management with
  client connection pooling and per-mirror routing.
- **`internal/service/repositorygate/`**: Bounded striped read/write gates that serialize repository engine and storage
  configuration changes with uploads, deletes, GPG publication, npm publish/dist-tag mutations, Docker manifest
  publication, review decisions, and mirror cache commits.
- **`internal/service/storage/` & `internal/service/gpg/`**: Multi-backend storage (Disk/S3), OpenPGP signature
  verification, and quarantined publication queue (`.renop.tmp.gpg`). The independent `files` repository format
  provides unstructured replaceable file storage and mirrors without checksum generation or signature processing.
  Browser navigation classifies indexed artifacts before format and authorization SPA branches, so a known file path
  never receives the SPA shell; Brotli, gzip, Zstandard, and the other supported compressed formats receive explicit
  binary MIME types without HTTP content-encoding labels. Maven files awaiting publication review are committed but
  blocked from every index insertion path; startup restores those blocks from the review database before watchers run,
  and GPG-success cleanup retains the publication block until an approval exposes or a rejection deletes the files.
  Virtual Docker review manifests are identified from their task type and bypass filesystem block, delete, and reindex
  operations while remaining downloadable through the authenticated review API.
- **`internal/service/packagestore/`**: Shared streaming package-blob boundary used by protocol modules for bounded
  staging, validation, atomic Disk/S3 commit, rollback, and deletion without importing storage implementations.
- **`internal/service/message/`**: Durable user message-center API for workflow events, team invitations, and
  administrator notices. Package-team removals create operator-neutral notifications localized by
  `internal/service/frontend/renop-html/js/team-messages.js`; scheduled and interactive system-update results are
  deduplicated per administrator and localized by `js/updater-messages.js` instead of transient dashboard prompts.
- **`internal/service/superteam/`**: Authenticated global-team account APIs for creation, pagination, immutable-prefix
  metadata, T1-T4 invitations and membership administration, effective account limits, and administrator overrides.
  T3 may manage T1/T2 members, while only T4 or system administrators may grant or manage T3/T4 roles; at least one
  T4 owner must remain. Administrators still enforce the target account's membership limit and receive no notification
  when adding themselves.
- **`internal/service/review/`**: Session-only, message-center-integrated review APIs for bounded reviewer/requester
  pages and global-team ownership transfers. Docker images, npm packages, Cargo crates, Maven artifacts, and Maven
  publishing domains share one stable resource model. An L4 owner or authorized administrator submits a request, a
  T3/T4 manager of the reviewing team or a system administrator decides it exactly once, and approval rechecks the
  requester's current L4 or administrator authority, target-team membership, and resource binding in the decision
  transaction. Namespaced Docker images and scoped npm packages cannot return to personal ownership.
  Maven publication reviews use repository-scoped moderators, a five-second last-file settling window, preset or
  bounded custom rejection reasons, authorized hidden-file streaming, and a single decision gate. Approval records
  catalog metadata before reindexing files; rejection removes the committed Disk/S3 objects. Repository configuration,
  migration, and deletion are rejected while a publication review remains pending.
  npm approvals use the same repository gate, payload lifecycle, file download API, preset rejection reasons, and
  single-decision contract as Maven while keeping protocol packuments unaware of pending versions.
  Docker publication tasks expose a validated virtual `manifest.json` through the same bounded file API without adding
  nonexistent objects to the storage index; approval completes its catalog and task mutation in one database transaction.
  npm and Docker creation requests use the reserved `@create` review version, bind retries to the requester's immutable
  identity, expose a bounded virtual JSON request, and create the resource in the same transaction as the review CAS.
  T2 global-team members always start with a T3/T4 team stage. When repository creation review is enabled, team approval
  atomically clears `review_team_prefix` on the same still-pending task and forwards it to repository moderators; the
  immutable `target_team_prefix` proves that team approval occurred, freezes later payload retries, and permits the
  final transaction to recheck the requester's live T2 membership without weakening direct T3/T4 creation.
- **`internal/service/reviewnotify/`**: Review lifecycle notification coordinator. New tasks send recipient-scoped,
  deduplicated notices to reviewers assigned to the current stage and system administrators. A T2 creation transition
  removes team notices before creating repository-moderator notices; one completed decision removes every remaining
  reviewer notice and sends the requester a result payload that never includes reviewer identity. Notification failures
  are recorded without rolling back an already durable task.
- **`internal/service/publicationquota/`**: Shared daily, weekly, or monthly publication accounting for users and global
  teams. Defaults are 600 files, 40 MiB, and 20 completed publications per month. Manager-only owner overrides may set
  individual limits to zero or enable the hidden no-quota flag. Validated uploads use short-lived durable reservations;
  team-bound packages and Maven domains consume only the owning team's quota, while mirrors never consume quota.
- **`internal/service/audit/`**: Durable behavior logging with a central registry of stable action identifiers.
  Frontend tests require every registered action to have a translation in every locale before changes can ship.
- **`internal/service/tasks/`**: Process-wide non-reentrant scheduler for coalescible periodic maintenance, including
  status snapshots, cache/session/global-team-invitation cleanup, index persistence, download-statistics flushing,
  publication-quota reservation and old-window cleanup, upload cleanup, and update checks.
  Event-driven workers such as audit persistence, GPG publication, token operations, and file watching remain
  dedicated and serial where ordering matters.
- **`internal/service/statistics/`**: Application-scoped bounded download counter shared by Maven, npm, Cargo, Docker, and
  unstructured files. One scheduler task flushes aggregates keyed by immutable user identity, repository, publishing
  domain, package/image, and version into `download_statistics`; checksum, signature, metadata, Javadoc, failed, HEAD,
  and noninitial range requests are excluded. Bearer API-token-only query routes provide bounded hierarchical pages,
  while repository settings expose enable/disable and atomic pending-plus-persisted reset controls. Docker image pull
  counts are updated from the same transaction for compatibility.
- **`internal/service/index/`**: Concurrent Disk/S3 repository index with deterministic children, snapshots, and
  negative-cache state. A normalized path is exclusively a file or a directory; authoritative file insertion removes
  stale directory state and descendants so API traversal cannot expose a file as an empty folder.
- **`internal/service/updater/`**: Authenticated update checking and installation with SHA-256 verification, bounded
  streaming decode of new raw `.br` executable packages, compatibility decode for legacy `.zip` packages, and
  deduplicated administrator result notifications. Download-start and imminent-restart progress remain transient
  frontend toasts; stable updater error codes localize online and offline failures without exposing filesystem details.
  Update results aggregate every retained release note between the
  running build and target; embedded full commit IDs and each stable record's `previous_commit` preserve ordering when
  version labels or older hosted records are unavailable.
- **`internal/middleware/` & `internal/api/`**: Format-aware search (modern Maven domain/artifact catalog,
  classic Maven/files index, and npm/Cargo/Docker package catalogs), anomaly detection, and brute-force mitigation.
- **`internal/daemon/`**: Cross-platform system service installation and lifecycle management (`--install`,
  `--uninstall`) supporting Windows Services (SCM), Linux (systemd & OpenRC), macOS (LaunchDaemons), and BSD (rc.d).
- **`internal/caddy/`**: Transactional `--install-caddy` deployment integration. It discovers standard Caddyfile and
  binary locations, validates normalized hostnames, updates one idempotent managed reverse-proxy block, constrains the
  RenoP listener to loopback, synchronizes the public domain, disables origin TLS, validates through Caddy, atomically
  replaces each file, and rolls both back if reload fails. Explicit paths and offline `--skip-reload` operation remain
  available for nonstandard service layouts.
- **`internal/utils/`**: Runtime memory/GC tuning (`InitMemoryTuning` for Linux/Windows) and process-wide string
  interning (`unique.Make`).
- **`web/` & `internal/service/frontend/`**: Embedded SPA with username-based `/user/<username>` profile, edit, and
  package-membership routes plus shared nickname-first identity components. Maven repositories use a domain catalog
  by default and can switch to the classic file-tree presentation. Repository catalogs list only domains containing
  artifacts in that repository, while global Maven domain and team configuration lives in the signed-in account menu.
  The account menu opens the routed `/account/maven-domains` subpage, whose server-backed multi-select permission/source
  filters and pagination keep large domain registries bounded.
  The signed-in account menu owns profile navigation, messages, logout, Maven domains, global teams, reviews,
  administrator pages, and the standalone administrator notification composer; the settings UI groups the server,
  outbound-proxy, and storage APIs under one Service domain. Administrators can configure a write-only GitHub OAuth
  secret there; GitHub login and
  profile linking request account/organization read access, persist immutable provider IDs without access tokens, and
  allow recently authorized account or organization identities to verify matching `io.github` Maven domains.
  Database ownership uses immutable user IDs, which remain hidden from the visible interface.
  Singular own-profile responses embed the private GitHub connection snapshot so the responsive two-column profile
  editor can render authorization state with the identity payload instead of issuing a delayed follow-up request;
  public and batched profile responses omit that connection data. OAuth redirects refresh the same snapshot, while
  disconnects update the visible state immediately and retain the server-side alternate-login invariant.
  Administrator account creation and editing use the responsive two-column `js/users/modal.js` dialog, with account
  identity and password semantics separated from the asynchronously loaded repository permission editor in
  `js/users/permissions.js`. Per-repository view, moderate, and deploy chips map to distinct permissions; moderator
  roles never expose manager-only tabs. Narrow viewports stack both sections without allowing the modal to exceed the
  dynamic viewport. The legacy protobuf `secret` field remains a transport-only compatibility detail and is not exposed as
  account-token terminology in the interface.
  Private email, password-login policy, and one-time recovery-code controls are isolated in
  `js/account-security.js` inside a default-collapsed, width-contained security card, while the public four-code reset
  workflow lives in `js/password-recovery.js`. The login dialog keeps password recovery as a secondary link and groups
  Passkey and optional GitHub controls in one provider section below the `or` divider; visible copy uses Passkey while
  stable FIDO/WebAuthn routes and audit identifiers remain unchanged.
  `js/api-tokens.js` owns the bounded token manager, scope selection, expiration, one-time secret display, shared
  clipboard feedback, immediate revocation, and live language refresh without exposing stored credential material.
  Its repository/package/team/domain target editors and parameterized errors share the height-animation primitives;
  the creation dialog grows naturally until the viewport clamp takes over, then keeps the footer fixed and body
  scrollable.
  `js/response-errors.js` is the shared boundary for user-facing HTTP failures: it reads only bounded error bodies,
  accepts registered stable codes or known localized messages, maps common statuses, and never exposes unknown backend
  text or runtime exception strings in the UI. `js/api.js` treats 401 as invalid authentication by default, while 403
  remains an ordinary authorization result unless a caller explicitly opts into logout; concurrent permission failures
  therefore cannot clear a valid browser session or start a route-reset loop.
  `js/main.js` is the single owner of browser `popstate` dispatch and home-route resets to prevent concurrent route
  loads. Protected account loaders signal the route boundary through `js/protected-route.js`; session expiry and
  permission denial replace the current history entry exactly once before any logout-triggered tab refresh, while
  explicit SPA routes bypass anonymous anomaly throttling without exempting their API calls. Valid authenticated 403
  responses never increment credential-failure counters. `js/reviews.js` owns the routed `/account/reviews` center,
  shared cross-engine transfer dialog, multi-type
  filtering, requester/reviewer views, pagination, and responsive height animation without using the message center.
  The same center downloads Maven review files with at most four adaptive workers, retries failures twice, assembles
  successful sets into a browser-side ZIP with a lazily loaded `fflate`, and falls back to direct critical-file
  downloads rather than emitting an incomplete archive. Maven and npm repository settings expose `off`,
  new-package-only, and every-version review policies through a separate JSON settings route so the legacy repository
  protobuf remains backward compatible.
  npm package-management responses add pending versions only for package members, repository moderators, and system
  administrators; npm protocol packuments and tarball routes continue to expose approved versions only.
  Modular i18n catalogs are split into common, auth/error, browser, management, messages/team, review, settings/updater,
  profile, repository, and package-format fragments under `js/i18n/<locale>/`. `scripts/i18n-catalog.mjs` loads
  fragments in parallel and reports all missing/extra keys and placeholder drift against the English catalog during
  `pnpm run build:frontend`. Cargo, Docker, and Maven repository subpages share persistent view lookup, busy state,
  route-height/entrance animation, back navigation, and timestamp adaptation through `js/browser/repository-view.js`.
  Entrance state is prepared before replacement nodes can paint, while Maven-domain filtering preserves its toolbar
  and filter shell and morphs only the bounded results/pagination region;
  repository clipboard feedback is centralized in `js/browser/copy-feedback.js`; repository package and namespace
  metadata grids and cross-format mirror-source badges are built by `js/browser/repository-view.js`.
  The routed `/account/teams` center owns global-team pagination, immutable-prefix creation, responsive T1-T4 member
  controls, shared username suggestions, invitation actions, and embedded profile usage limits. System settings load
  global team defaults through a separate JSON domain without expanding the protobuf settings schema; all 12 frontend
  locales include global-team UI, message, error, and audit text. The shared selector exposes T2+ teams for Docker/npm
  creation and T3+ teams for Maven-domain creation; namespace validation, live role checks, ordered approval stages,
  and API-token `global/<prefix>` targets are rechecked server-side before the transactional reservation.
  Maven artifact versions, npm package versions, and Docker image tags use `@renop/ui/pagination` for bounded
  previous/next pages, responsive summaries, height-morphed page changes, and page clamping after deletions; the
  shared pager intentionally avoids dense numbered-button rows on mobile.
  Cargo package collaborators and repository moderators can see pending versions in the package page, while public
  sparse-index and catalog responses remain unchanged until approval.
  Docker image collaborators and repository moderators see pending tags with pull, inspect, and delete actions withheld;
  public Registry v2 manifest, tag-list, and catalog responses remain unchanged until approval.
  Approved npm/Docker creation dialogs enter the review center through its application-level router, avoiding a stale
  repository-loader error before refresh.
  `js/review-messages.js` localizes pending and result notifications from stable payload fields. Review list and decision
  responses redact `decided_by` for requesters and non-system moderators; only system administrators receive it.
  `js/publication-quota.js` renders the same responsive usage component in the own-profile editor, administrator user
  actions, and global-team details. System settings provide global defaults; only system administrators receive the
  owner editor and no-quota control. Every protocol maps stable quota errors without exposing backend text.
  npm repositories use `js/browser/npm.js` for bounded catalog, package, integrity, immutable-version, dist-tag,
  visibility, provenance, published README/project metadata, and responsive L0-L4 team management while protocol
  failures are localized through stable codes in `js/npm-errors.js`. npm integrity/action panels and Maven version-file
  panels use the reversible shared disclosure controller; description editors initialize textarea values through DOM
  properties, and their common save action is localized from the shared catalog. Package-controlled Markdown flows through the
  inert element-and-URL allowlist in `js/markdown.js` and the shared neutral layout in
  `css/components/markdown.css`; protocol views never assign rendered Markdown directly to an active element.
  `js/repository-formats.js` owns canonical per-engine icons, `js/repository-list.js` owns deterministic repository
  name ordering plus engine filtering and bounded pagination, while `js/components/icon.js` maps detailed file types
  into a bounded set of shared visual families. The top navigation and routed application content share one
  border-box shell width and gutter, keeping the home brand aligned with page content at desktop widths. npm, Cargo,
  and Docker team invitations
  share the keyboard-accessible, viewport-aware `js/browser/user-suggestions.js` controller and component stylesheet.
  All frontend clipboard writes and seconds/milliseconds/ISO timestamp normalization flow through `js/clipboard.js`
  and `js/time.js`. The server-rendered H5 shell is cached at bootstrap and regenerated atomically after frontend
  settings updates. Its validated font preset metadata selects a shared system-ui baseline that keeps Linux text
  metrics aligned; `js/font.js` loads direct font files or Google Fonts CSS endpoints during idle time and activates
  the resolved family only after it becomes usable, so fonts never enter the render-blocking stylesheet path.
  `@renop/ui/jquery` owns the official self-hosted jQuery 4 runtime shared by the embedded frontend and website;
  Rolldown emits it as a hashed module chunk, installs `jQuery` and a collision-safe `$` before interactive startup,
  and emits `jqueryReady` without a Migrate shim. jQuery owns document-ready/delegated event wiring plus the shared
  DOM, modal, theme, tabs, language-card, toggle, custom-select, and website-router layers; streaming fetches,
  observers, pointer-capture details, and Web Animations remain native where jQuery would weaken security or
  performance. `@renop/ui/disclosure` composes the height-animation layer with accessible details/summary semantics
  and supports rapid direction reversal. Unstructured files repositories suppress the protocol-specific repository
  snippet card while retaining storage and mirror statistics. Shared
  select controls pair `@renop/ui/custom-select` with its canonical package stylesheet; native
  option popups are not used for styled application dialogs. The i18n runtime incrementally translates asynchronously inserted declarative UI nodes, while
  shared modal CSS clamps dialogs to the dynamic viewport and device safe areas. Shared asynchronous actions use the
  button-state helper exported by `js/components/button.js`, which restores controls after both successful and failed
  requests. `js/backend-availability.js` confirms same-origin request failures with foreground health probes so browser
  suspension cannot produce a stale blocking offline state. Docker management
  failures expose stable `X-Renop-Error-Code` values that `js/docker-errors.js` maps to the Docker locale catalogs;
  browser and message-center views never display raw backend error text. The official website Markdown renderer derives
  heading labels and anchors from visible inline-token text so emphasis delimiters are not repeated in the docs TOC.
  The website download page recognizes raw Brotli update targets and uses the separately bundled
  `js/update-package-worker.js` with `brotli-compress/js` and `fflate` to perform SHA-256-verified, adaptively parallel
  downloads and optional in-browser conversion to the legacy ZIP layout with at most four workers.
  Website documentation uses `en-US` as the canonical contract. `web/test/docs-parity.test.mjs` requires every supported
  documentation locale to preserve the same file set, heading outline, fenced examples, HTTP endpoints, local links,
  and a minimum non-abbreviated content size before `pnpm run test:web` can pass.

---

## 2. Environment & Essential Commands

### Toolchains

- **Go**: Go 1.28+ (`404Setup/go` fork)
- **Frontend**: Node.js 18+ with **pnpm**
- **Shell**: PowerShell 7 (`pwsh`)
- **Protobuf**: `protoc` with `protoc-gen-go`
- **Release packaging**: `build.ps1` automatically installs `cmd/renop-brotli` into the active Go binary directory;
  packaged builds emit raw `.br` executable streams while `nb` builds remain unpackaged binaries. Target
  compilation is bounded by `-BuildConcurrency` (default and maximum 4), while independent packaging is bounded by
  `-CompressionConcurrency` (default and maximum 8). Formal builds embed the current and previous release commits and
  publish both values through `manifest.json` and channel `info.json`.

### Build & Test Workflows

| Task                                                | Command                                                                          |
|-----------------------------------------------------|----------------------------------------------------------------------------------|
| **Local Dev Build** (unzipped binary)               | `pwsh ./build.ps1 c nb`                                                          |
| **Packaged Release Build** (current OS, raw Brotli) | `pwsh ./build.ps1 c`                                                             |
| **Full Matrix Release Build** (raw Brotli)          | `pwsh ./build.ps1`                                                               |
| **Website & Docs Build**                            | `pnpm run build:web`                                                             |
| **Website Markdown Tests**                          | `pnpm run test:web`                                                              |
| **Frontend Build & Embed**                          | `pnpm install --frozen-lockfile && pnpm run build:frontend && go generate ./...` |
| **Frontend Unit Tests**                             | `pnpm run test:frontend`                                                         |
| **Frontend i18n Validation**                        | `pnpm run check:i18n`                                                            |
| **Protobuf Generation**                             | `protoc -I proto --go_out=. --go_opt=module=renop proto/api/v1/api.proto`        |
| **Run All Tests**                                   | `go test ./...`                                                                  |
| **Run Package Tests**                               | `go test -v ./internal/...`                                                      |
| **Database Driver Contract**                       | `go run ./cmd/renop-dbtest -driver <driver> -dsn <isolated-dsn> -confirm-isolated` |
| **Install as Service**                              | `./renop --install`                                                              |
| **Uninstall Service**                               | `./renop --uninstall`                                                            |
| **Configure Caddy**                                 | `./renop --install-caddy --hostname renop.example.com`                           |

---

## 3. Strict AI Rules & Guidelines

### 1. Task Planning & Atomic Execution

- **Pre-Execution Analysis & Planning**: When receiving multiple tasks or complex requirements, analyze dependencies
  upfront. Determine whether related tasks should be merged into a single cohesive change, structure an explicit
  execution plan, and execute systematically.
- **Atomic Change Units**: Complete each logical task end-to-end—including all related fixes, error handling, and test
  updates. Do NOT fragment a single logical change into disjointed micro-edits or half-baked steps.
- **Regression Ownership**: Any bug or regression introduced by a change must be investigated and resolved within the
  same task scope before considering the task complete.

### 2. Code Comments & Documentation Standards

- **English-Only Requirement**: All comments, docstrings, and documentation across the entire codebase MUST be written
  in **English only**. Non-English comments are strictly forbidden.
- **Standardized Doc Comments**:
    - Exported Go types, structs, interfaces, functions, and methods must have standard Go doc comments
      (`// TypeName ...` or `// FunctionName ...`).
    - Manually written JavaScript/TypeScript functions must include standard JSDoc (`/** ... */`).
- **Zero Junk / Clutter Comments**:
    - Do NOT clutter method bodies with redundant, obvious, or low-value inline comments (e.g., `// parse json`,
      `// return result`, `// check error`).
    - Code must be self-explanatory by default. Inline comments should only exist to explain non-obvious business
      rationale, safety constraints, or complex algorithmic details ("why", not "what").

### 3. Engineering & Quality Standards

- **Mandatory Verification**: ALWAYS run `go test ./...` or `pwsh ./build.ps1 c nb` to verify code changes before
  completion. Never claim success without empirical test/build execution output.
- **No Blind Assumptions**: ALWAYS inspect actual types, signatures, and imports in the codebase before editing.
- **Robust Error Handling**: Never swallow errors silently, return dummy fallbacks, or delete failing assertions.
- **Performance Safety**: Avoid unnecessary heap allocations and redundant copies in hot paths (streaming, hashing,
  database rebinding, file indexing).
- **Keep AGENTS.md Updated**: Always update `AGENTS.md` if any change affects architecture, toolchains, build scripts,
  or workflows.
