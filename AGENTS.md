# AGENTS.md

> **CRITICAL MANDATE FOR ALL AI ASSISTANTS**:
> Whenever you modify project architecture, toolchains, build scripts, workflows, or directory structures, **you MUST
update this `AGENTS.md` file in the same turn** to ensure future AI agents receive up-to-date context.

---

## 1. Project Overview & Architecture

**RenoP** is a self-hosted package repository server with a **Go** backend and an embedded **Node.js/pnpm** frontend.

- **`server.go`**: Server main entry point.
- **`internal/`**: Server core logic (HTTP routes, auth, Maven repository proxying, storage adapters for S3/Local Disk).
- **`internal/database/`**: Pluggable database layer supporting **SQLite**, **MySQL**, and **PostgreSQL** (via `github.com/jackc/pgx/v5` stdlib driver `pgx`). Dialects encapsulate schema creation, shared column migrations, and dialect-specific upsert queries (`ON CONFLICT` / `ON DUPLICATE KEY UPDATE`). A zero-alloc lexer `RebindPostgres` translates standard `?` positional parameters into `$1, $2...` while safely ignoring string literals, identifiers, and comments. `*database.DB` and `*database.Tx` provide uniform query execution with automatic parameter rebinding.
- **`internal/service/proxy/client.go`**: Bounded cache of HTTP clients for named global mirror-proxy selections.
  Mirrors store only a selector (`""` inherits global state, `direct` bypasses it); direct traffic reuses the shared
  outbound client.
- **`internal/service/outboundproxy/`**: Validates the Settings-level named HTTP/HTTPS/SOCKS5 proxy list and configures
  dedicated outbound transports for selected global or mirror-specific routing. GPG key resolution and mirror requests
  share the validated global proxy model.
- Settings expose `frontend`, `server`, `proxy`, `storage`, `updater`, and `index` domains. GPG key-server configuration
  is embedded in `ServerConfig.gpg`; the old standalone `gpg` settings domain is not served.
- Mirror authentication supports validated custom request headers while reusing `MirrorCredentials.login` for the header
  name and `password` for its token; routing and hop-by-hop headers are rejected before persistence.
- Maven metadata requested through the API is fetched through the configured mirror with a bounded response size and an
  in-flight request lock; cached version-level metadata drives cleanup of superseded Maven SNAPSHOT builds, including
  numeric and timestamped build forms.
- **`internal/service/cargo/`**: Sparse-only Cargo registry protocol, bounded streaming publication and archive
  validation, upstream index/artifact mirroring, package lifecycle operations, and L1-L3 collaboration managed through
  message-center invitations. Repository format is selected at creation and is immutable; format-specific settings and
  browser behavior are defined by the frontend repository-format registry so future package protocols can be added
  without branching the entire repository UI. Repository JSON/YAML serialization omits Maven-only publication policy
  from Cargo entries and strips Cargo artifact URL templates from Maven entries.
- **`internal/api/search.go`**: Bounded, format-aware repository search. Maven queries the in-memory file index with
  path-permission and quarantine filtering; Cargo queries public package metadata and every result links to a readable
  package subpage. The frontend renders results in a body-attached overlay so searching never changes the repository
  browser's dimensions.
- **`internal/service/gpg/`**: OpenPGP key resolution and detached-signature verification. User profiles may register up
  to 10 public-key IDs; HTTPS key-server lookups are response-bounded, validate every resolved address as public, use
  bounded IPv4-first address fallback for direct connections, can use the selected global proxy, and cache keys in the
  database.
- **`internal/service/message/`**: Durable user message-center API. Feature modules deliver typed, user-scoped messages
  with server-owned action kinds (never callback URLs); users can read, remove completed notifications, or clear all
  dismissible notifications without losing pending workflow requests, while managers
  can send bounded plain-text notices to named users or all users. A manager-only, prefix-bounded username search powers
  recipient autocomplete without exposing token secrets. The embedded frontend polls only the unread count and provides
  the matching localized message center and manager composer.
- **`internal/service/storage/gpg_*.go`**: Durable, single-threaded GPG publication queue for `.jar`, `.pom`, and
  `.module` uploads. Pending files live under the private `.renop.tmp.gpg` quarantine, remain persistently blocked from
  the file index across rebuilds/restarts, and are published only after verification. Terminal state and failure reasons
  are exposed in the user's profile publication history; storage-path changes are rejected while publication or cleanup
  work remains.
- **`internal/service/status/process_memory_*.go`**: Platform-specific process memory sampling. Linux reuses
  `/proc/self/statm` with a fixed buffer; Windows uses process counters; other supported systems use the platform
  adapter from gopsutil.
- **`internal/utils/memory_*.go`**: Cross-platform memory and GC tuning (`InitMemoryTuning`). Linux resolves cgroup v1/v2 limits or `/proc/meminfo` to configure `debug.SetMemoryLimit` and disable transparent huge pages; Windows queries `GlobalMemoryStatusEx` for soft memory bounding; all platforms tune GC percent to 35% when `GOGC` is unset.
- **`internal/utils/intern.go`**: Process-wide string interning using Go's `unique.Make` for zero-leak canonical path string deduplication across `FileIndex` and cached mappings.
- **`pkg/`**: Public/shared Go libraries.
- **`proto/`**: Protocol Buffers schema definitions (`proto/api/v1/api.proto`).
- **`web/`**, **`packages/`**, & **`internal/service/frontend/renop-html/`**: Frontend web UI and workspace packages
  (managed via `pnpm`).
- Frontend API requests use an explicit `no-store` fetch policy. Entry HTML is never stored, while versioned embedded
  assets retain revalidation headers on both `200` and conditional `304` responses so stale browser cache state cannot
  leave the single-page UI waiting on code that no longer matches the server.
- **`internal/service/frontend/renop-html/js/i18n/<locale>/`**: Modular frontend translations. Each locale contains
  the same feature fragments (for example `core.js`, `repository.js`, and `cargo.js`); `build.mjs` validates locale/key parity and
  generates `js/i18n/catalog.generated.js`. Run the frontend build instead of editing the catalog directly.
- Cargo repositories render package-centric routes at `/<repository>/packages[/<package>]` in the main browser column;
  they never expose Maven's raw file-tree management view. The overview is a bounded, paginated public package catalog
  that merges management state only for authorized sessions. Package metadata and versions are readable under normal
  repository visibility rules, while team membership and mutation controls remain collaborator/administrator-only.
  Cargo subpage navigation preserves the previous content and reuses immutable repository-format metadata until the
  next route is ready. Package-team autocomplete and permission menus are body-attached or fixed-width controls to
  avoid resizing their page containers. Repository settings select Maven versus Cargo mirror URL labels and hints from
  the immutable format rather than presenting protocol-specific fields globally.
- **`build.ps1`**: PowerShell 7 build script for single and cross-platform builds. Linux targets link
  `runtime.godebugDefault=disablethp=1` so transparent huge pages are disabled before the first Go heap mapping;
  operators can override this with `GODEBUG=disablethp=0`.
- **`.github/actions/setup-go-runtime/`**: Composite Action that resolves, verifies, installs, and caches the prebuilt
  custom Go runtime from `404Setup/go` releases.

---

## 2. Environment & Toolchains

- **Go**: Go 1.28+ from the `404Setup/go` fork. CI reads the required version from the `go` directive in `go.mod` unless
  the setup Action receives an explicit version, then selects the newest matching release prefix (including
  prerelease/development tags).
- **Node.js**: Node.js 18+ with **pnpm**
- **PowerShell**: PowerShell 7 (`pwsh`)
- **Protobuf Compiler**: `protoc` (with `protoc-gen-go` plugin)
- **OpenPGP**: Backend verification uses `github.com/ProtonMail/go-crypto`; no browser-side signing dependency is used.
- **Database Drivers**: Modern pure Go drivers — SQLite (`modernc.org/sqlite`), MySQL (`github.com/go-sql-driver/mysql`), and PostgreSQL (`github.com/jackc/pgx/v5`).

---

## 3. Essential Commands & Workflows

### Build Commands

- **Quick Local Build (Current OS, unzipped binary `renop.exe` or `renop`)**:
  `pwsh ./build.ps1 c nb`
- **Current OS Build (packaged ZIP archive)**:
  `pwsh ./build.ps1 c`
- **Full Release Matrix Build**:
  `pwsh ./build.ps1`
- **Frontend Build & Embed**:
  ```powershell
  pnpm install --frozen-lockfile
  pnpm run build:frontend
  go generate ./...
  ```
- **Protobuf Generation**:
  `protoc -I proto --go_out=. --go_opt=module=renop proto/api/v1/api.proto`
- **CI Go Runtime Setup**:
  `.github/actions/setup-go-runtime` installs the matching prebuilt release asset (`.tar.gz` on non-Windows runners,
  `.zip` on Windows), verifies its GitHub-provided SHA-256 digest, and sets `GOROOT`, `PATH`, and `GOTOOLCHAIN=local`.

### Testing & Verification

- **Run All Tests**:
  `go test ./...`
- **Run Package-Specific Tests**:
  `go test -v ./internal/...`

---

## 4. Strict AI Rules & Guidelines

1. **Mandatory Verification**: MUST run `go test ./...` or `pwsh ./build.ps1 c nb` to verify code changes before
   completion. NEVER claim success without empirical execution output.
2. **No Blind Assumptions**: ALWAYS inspect actual function signatures, types, and imports using file viewing tools
   before making edits.
3. **Error Handling & Quality**: NEVER swallow errors silently, return dummy fallbacks, or delete failing test
   assertions.
4. **Performance Safety**: Avoid unnecessary heap allocations or memory copies in hot execution paths (e.g., streaming,
   storage, hashing).
5. **Keep AGENTS.md Updated**: If your task alters build scripts, dependencies, project paths, or execution workflows,
   **you MUST update `AGENTS.md` before finishing**.
6. **JavaScript Documentation**: Every manually written or modified JavaScript function must include JSDoc. Generated
   JavaScript should remain generator-owned.
