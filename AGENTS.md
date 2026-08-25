# AGENTS.md

> **CRITICAL MANDATE**:
> Whenever you modify architecture, toolchains, build scripts, workflows, or directory structures, **update this
`AGENTS.md` in the same turn**.

---

## 1. Project Architecture & Core Modules

**RenoP** is a high-performance self-hosted package repository server with a **Go** backend and an embedded
**Node.js/pnpm** frontend.

- **`server.go`**: Application entry point and server lifecycle.
- **`internal/database/`**: Pluggable multi-dialect DB (SQLite, MySQL, PostgreSQL via `jackc/pgx/v5`). Includes
  zero-alloc SQL parameter rebinding (`RebindPostgres`), unified transaction wrappers, schema migrations, public user
  profiles, immutable user identities for package ownership, and durable username-change throttling.
- **`internal/service/cargo/` & `internal/service/cargodocs/`**: Sparse Cargo registry implementation, crate lifecycle,
  authoritative upstream name-conflict checks, upstream proxying, and sandboxed documentation extraction/viewer
  (`/cargodoc/...`).
- **`internal/service/maven/`**: Maven domain registry with DNS/GitHub/GitLab ownership verification, cross-repository
  proof reuse, L0-L4 domain teams, invitation workflows, catalog/version management, and legacy repository import.
  Maven repositories support modern domain-catalog and classic file-tree layouts while enforcing the same verified
  Maven publication paths in both layouts.
- **`internal/service/docker/`**: OCI & Docker Registry v2 specification implementation (`/v2/...`), token-based
  Bearer authentication, explicitly reserved images, L0-L4 image teams, per-image private visibility, image-scoped
  blob references, chunked uploads, authorized cross-repository mounting, upstream mirror proxying, and catalog
  management. Client pushes cannot create images implicitly; administrators reserve public or private images through
  the frontend first. Local reservations are unique and cannot claim names exposed by an enabled upstream mirror;
  mirror-discovered images remain permanently pull-only.
- **`internal/service/proxy/` & `internal/service/outboundproxy/`**: Outbound HTTP/HTTPS/SOCKS5 proxy management with
  client connection pooling and per-mirror routing.
- **`internal/service/storage/` & `internal/service/gpg/`**: Multi-backend storage (Disk/S3), OpenPGP signature
  verification, and quarantined publication queue (`.renop.tmp.gpg`). The independent `files` repository format
  provides unstructured replaceable file storage and mirrors without checksum generation or signature processing.
- **`internal/service/message/`**: Durable user message-center API for workflow events, team invitations, and
  administrator notices.
- **`internal/middleware/` & `internal/api/`**: Format-aware search (Maven file index vs. Cargo package catalog),
  anomaly detection, and brute-force mitigation.
- **`internal/daemon/`**: Cross-platform system service installation and lifecycle management (`--install`, `--uninstall`) supporting Windows Services (SCM), Linux (systemd & OpenRC), macOS (LaunchDaemons), and BSD (rc.d).
- **`internal/utils/`**: Runtime memory/GC tuning (`InitMemoryTuning` for Linux/Windows) and process-wide string
  interning (`unique.Make`).
- **`web/` & `internal/service/frontend/`**: Embedded SPA with username-based `/user/<username>` profile, edit, and
  package-membership routes plus shared nickname-first identity components. Maven repositories use a domain catalog
  by default and can switch to the classic file-tree presentation. The signed-in account menu owns profile
  navigation and administrator page entry points; the settings UI groups the server, outbound-proxy, and storage APIs
  under one Service domain. Database ownership uses immutable user IDs, which remain hidden from the visible interface.
  `js/main.js` is the single owner of browser `popstate` dispatch to prevent concurrent route loads. Modular i18n
  catalogs, including dedicated profile fragments, live under `js/i18n/<locale>/` and are compiled via
  `pnpm run build:frontend`. Shared asynchronous actions use the button-state helper exported by
  `js/components/button.js`, which restores controls after both successful and failed requests. Docker management
  failures expose stable `X-Renop-Error-Code` values that `js/docker-errors.js` maps to the Docker locale catalogs;
  browser and message-center views never display raw backend error text. The official website Markdown renderer derives
  heading labels and anchors from visible inline-token text so emphasis delimiters are not repeated in the docs TOC.

---

## 2. Environment & Essential Commands

### Toolchains

- **Go**: Go 1.28+ (`404Setup/go` fork)
- **Frontend**: Node.js 18+ with **pnpm**
- **Shell**: PowerShell 7 (`pwsh`)
- **Protobuf**: `protoc` with `protoc-gen-go`

### Build & Test Workflows

| Task                                    | Command                                                                          |
|-----------------------------------------|----------------------------------------------------------------------------------|
| **Local Dev Build** (unzipped binary)   | `pwsh ./build.ps1 c nb`                                                          |
| **Packaged Release Build** (current OS) | `pwsh ./build.ps1 c`                                                             |
| **Full Matrix Release Build**           | `pwsh ./build.ps1`                                                               |
| **Website & Docs Build**               | `pnpm run build:web`                                                             |
| **Website Markdown Tests**             | `pnpm run test:web`                                                              |
| **Frontend Build & Embed**              | `pnpm install --frozen-lockfile && pnpm run build:frontend && go generate ./...` |
| **Protobuf Generation**                 | `protoc -I proto --go_out=. --go_opt=module=renop proto/api/v1/api.proto`        |
| **Run All Tests**                       | `go test ./...`                                                                  |
| **Run Package Tests**                   | `go test -v ./internal/...`                                                      |
| **Install as Service**                  | `./renop --install`                                                              |
| **Uninstall Service**                   | `./renop --uninstall`                                                            |

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
