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
- **`scripts/build-target.ps1` & `scripts/compress-target.ps1`**: Isolated release workers coordinated by `build.ps1`.
  Up to four compilations run independently from up to eight Brotli packaging tasks; a completed compilation releases
  its build slot immediately and queues compression without delaying the next architecture. The parent preserves
  deterministic manifest order and aggregates failures from both pools. The `dist/` update payload is restricted to
  raw `.br` packages plus `manifest.json`; `.github/scripts/test-release-payload.ps1` enforces that boundary before
  the update API is called. License, README, and third-party notices are attached to GitHub releases directly from the
  checkout and are never uploaded to the update API.
- **`internal/database/`**: Pluggable multi-dialect DB (SQLite, MySQL, PostgreSQL via `jackc/pgx/v5`). Includes
  zero-alloc SQL parameter rebinding (`RebindPostgres`), unified transaction wrappers, schema migrations, public user
  profiles, immutable user identities for package ownership, private normalized login emails, serialized login-method
  invariants, masked account-token/profile mutations, irreversible one-time recovery-code verifiers, and hashed,
  expiring fine-grained API credentials. Legacy plaintext upload tokens migrate transactionally to scoped hashes;
  durable GitHub identity/principal snapshots and username-change throttling remain bound to immutable user IDs.
- **`internal/service/auth/`**: Password, FIDO/Passkey, session, profile, and GitHub OAuth workflows. GitHub OAuth
  separates bounded single-use route state, constrained provider HTTP access, and collision-safe account linking into
  `github_routes.go`, `github_client.go`, and `github_account.go`; access tokens are never persisted. Account recovery
  uses twelve 160-bit codes, Argon2id verifiers, four-code atomic consumption, and session revocation; password login
  may be disabled only while a GitHub identity or Passkey remains available. API tokens use one-time 256-bit secrets,
  optional expiration, current-account-permission intersection, and immediate revocation. Capabilities separately
  gate repository reads/publication/deletion, package creation/metadata/lifecycle, team administration, and Maven-domain
  reading/creation/verification/deletion. Each target-aware scope can additionally carry bounded exact repository,
  package, team, or domain restrictions in the backward-compatible authorization JSON; legacy broad package/domain
  scopes remain authentication-only compatibility.
  Token secrets are owner-managed from a browser session; administrators cannot mint credentials for another user.
  Browser session secrets are cookie-only, while Basic/password credentials are restricted to package protocols.
- **`internal/service/cargo/` & `internal/service/cargodocs/`**: Sparse Cargo registry implementation, crate lifecycle,
  authoritative upstream name-conflict checks, mirrored-crate provenance, upstream proxying, and sandboxed documentation
  extraction/viewer (`/cargodoc/...`).
- **`internal/service/maven/`**: Process-wide Maven domain registry with DNS/GitHub/GitLab ownership verification,
  global L0-L4 domain teams shared by every Maven repository, invitation workflows, catalog/version management, and
  automatic migration of repository-scoped legacy domains. Upstream mirror discovery persists unverified global
  domains so administrators can filter, inspect, and explicitly approve them. Maven and Cargo mirror downloads are
  cataloged through
  the format-aware proxy completion hook in `internal/service/storage/mirror.go` without buffering artifact bodies.
  Maven repositories support modern domain-catalog and classic file-tree layouts while enforcing the same verified
  Maven publication paths in both layouts. Administrators can migrate Maven repositories to the unstructured files
  engine and back without moving stored objects; returning to Maven streams the existing Disk/S3 index into a rebuilt
  catalog and restores the prior Maven layout and publication policy.
- **`internal/service/docker/`**: OCI & Docker Registry v2 specification implementation (`/v2/...`), token-based
  Bearer authentication, explicitly reserved images, L0-L4 image teams, per-image private visibility, image-scoped
  blob references, chunked uploads, authorized cross-repository mounting, upstream mirror proxying, and catalog
  management. Client pushes cannot create images implicitly; administrators reserve public or private images through
  the frontend first. Local reservations are unique and cannot claim names exposed by an enabled upstream mirror;
  mirror-discovered images remain permanently pull-only.
- **`internal/service/proxy/` & `internal/service/outboundproxy/`**: Outbound HTTP/HTTPS/SOCKS5 proxy management with
  client connection pooling and per-mirror routing.
- **`internal/service/repositorygate/`**: Bounded striped read/write gates that serialize repository engine and storage
  configuration changes with uploads, deletes, GPG publication, and mirror cache commits.
- **`internal/service/storage/` & `internal/service/gpg/`**: Multi-backend storage (Disk/S3), OpenPGP signature
  verification, and quarantined publication queue (`.renop.tmp.gpg`). The independent `files` repository format
  provides unstructured replaceable file storage and mirrors without checksum generation or signature processing.
  Browser navigation classifies indexed artifacts before format and authorization SPA branches, so a known file path
  never receives the SPA shell; Brotli, gzip, Zstandard, and the other supported compressed formats receive explicit
  binary MIME types without HTTP content-encoding labels.
- **`internal/service/message/`**: Durable user message-center API for workflow events, team invitations, and
  administrator notices. Package-team removals create operator-neutral notifications localized by
  `internal/service/frontend/renop-html/js/team-messages.js`; scheduled and interactive system-update results are
  deduplicated per administrator and localized by `js/updater-messages.js` instead of transient dashboard prompts.
- **`internal/service/audit/`**: Durable behavior logging with a central registry of stable action identifiers.
  Frontend tests require every registered action to have a translation in every locale before changes can ship.
- **`internal/service/tasks/`**: Process-wide non-reentrant scheduler for coalescible periodic maintenance, including
  status snapshots, cache/session cleanup, index persistence, pull-count flushing, upload cleanup, and update checks.
  Event-driven workers such as audit persistence, GPG publication, token operations, and file watching remain
  dedicated and serial where ordering matters.
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
  classic Maven/files index, and Cargo/Docker package catalogs), anomaly detection, and brute-force mitigation.
- **`internal/daemon/`**: Cross-platform system service installation and lifecycle management (`--install`,
  `--uninstall`) supporting Windows Services (SCM), Linux (systemd & OpenRC), macOS (LaunchDaemons), and BSD (rc.d).
- **`internal/utils/`**: Runtime memory/GC tuning (`InitMemoryTuning` for Linux/Windows) and process-wide string
  interning (`unique.Make`).
- **`web/` & `internal/service/frontend/`**: Embedded SPA with username-based `/user/<username>` profile, edit, and
  package-membership routes plus shared nickname-first identity components. Maven repositories use a domain catalog
  by default and can switch to the classic file-tree presentation. Repository catalogs list only domains containing
  artifacts in that repository, while global Maven domain and team configuration lives in the signed-in account menu.
  The account menu opens the routed `/account/maven-domains` subpage, whose server-backed multi-select permission/source
  filters and pagination keep large domain registries bounded.
  The signed-in account menu owns profile navigation, messages, logout, Maven domains, administrator pages, and the
  standalone administrator notification composer; the settings UI groups the server, outbound-proxy, and storage APIs
  under one Service domain. Administrators can configure a write-only GitHub OAuth secret there; GitHub login and
  profile linking request account/organization read access, persist immutable provider IDs without access tokens, and
  allow recently authorized account or organization identities to verify matching `io.github` Maven domains.
  Database ownership uses immutable user IDs, which remain hidden from the visible interface.
  Private email, password-login policy, and one-time recovery-code controls are isolated in
  `js/account-security.js` inside a default-collapsed, width-contained security card, while the public four-code reset
  workflow lives in `js/password-recovery.js`.
  `js/api-tokens.js` owns the bounded token manager, scope selection, expiration, one-time secret display, shared
  clipboard feedback, immediate revocation, and live language refresh without exposing stored credential material.
  `js/main.js` is the single owner of browser `popstate` dispatch and home-route resets to prevent concurrent route
  loads. Modular i18n
  catalogs are split into common, auth/error, browser, management, messages/team, settings/updater, profile,
  repository, and package-format fragments under `js/i18n/<locale>/`. `scripts/i18n-catalog.mjs` loads fragments in
  parallel and reports all missing/extra keys and placeholder drift against the English catalog during
  `pnpm run build:frontend`. Cargo, Docker, and Maven repository subpages share persistent view lookup, busy state,
  route-height/entrance animation, back navigation, and timestamp adaptation through `js/browser/repository-view.js`;
  repository clipboard feedback is centralized in `js/browser/copy-feedback.js`; repository package and namespace
  metadata grids and cross-format mirror-source badges are built by `js/browser/repository-view.js`.
  `js/repository-formats.js` owns canonical per-engine icons, `js/repository-list.js` owns deterministic repository
  name ordering plus engine filtering and bounded pagination, while `js/components/icon.js` maps detailed file types
  into a bounded set of shared visual families. Cargo and Docker team invitations
  share the keyboard-accessible, viewport-aware `js/browser/user-suggestions.js` controller and component stylesheet.
  All frontend clipboard writes and seconds/milliseconds/ISO timestamp normalization flow through `js/clipboard.js`
  and `js/time.js`. Shared select controls pair `@renop/ui/custom-select` with its canonical package stylesheet; native
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
| **Install as Service**                              | `./renop --install`                                                              |
| **Uninstall Service**                               | `./renop --uninstall`                                                            |

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
