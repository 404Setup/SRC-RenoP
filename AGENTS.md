# AGENTS.md

> **CRITICAL MANDATE FOR ALL AI ASSISTANTS**:
> Whenever you modify project architecture, toolchains, build scripts, workflows, or directory structures, **you MUST update this `AGENTS.md` file in the same turn** to ensure future AI agents receive up-to-date context.

---

## 1. Project Overview & Architecture

**RenoP** is a self-hosted Maven repository server with a **Go** backend and an embedded **Node.js/pnpm** frontend.

- **`server.go`**: Server main entry point.
- **`internal/`**: Server core logic (HTTP routes, auth, Maven repository proxying, storage adapters for S3/Local Disk).
- **`internal/service/proxy/client.go`**: Bounded per-mirror HTTP/SOCKS5 proxy client cache. Mirror proxy credentials and transports are isolated by configuration, while unconfigured mirrors retain the shared outbound transport.
- Mirror authentication supports validated custom request headers while reusing `MirrorCredentials.login` for the header name and `password` for its token; routing and hop-by-hop headers are rejected before persistence.
- **`internal/service/status/process_memory_*.go`**: Platform-specific process memory sampling. Linux reuses `/proc/self/statm` with a fixed buffer; Windows uses process counters; other supported systems use the platform adapter from gopsutil.
- **`internal/utils/memory_linux.go`**: Linux GC tuning. It resolves the process's cgroup v1/v2 mount and hierarchy, honors the strictest configured memory limit, leaves headroom for non-Go process memory, and applies a direct-build fallback for `GODEBUG=disablethp=1`. An explicit `disablethp` value in `GODEBUG` is preserved.
- **`pkg/`**: Public/shared Go libraries.
- **`proto/`**: Protocol Buffers schema definitions (`proto/api/v1/api.proto`).
- **`web/`**, **`packages/`**, & **`internal/service/frontend/renop-html/`**: Frontend web UI and workspace packages (managed via `pnpm`).
- **`build.ps1`**: PowerShell 7 build script for single and cross-platform builds. Linux targets link `runtime.godebugDefault=disablethp=1` so transparent huge pages are disabled before the first Go heap mapping; operators can override this with `GODEBUG=disablethp=0`.
- **`.github/actions/setup-go-runtime/`**: Composite Action that resolves, verifies, installs, and caches the prebuilt custom Go runtime from `404Setup/go` releases.

---

## 2. Environment & Toolchains

- **Go**: Go 1.28+ from the `404Setup/go` fork. CI reads the required version from the `go` directive in `go.mod` unless the setup Action receives an explicit version, then selects the newest matching release prefix (including prerelease/development tags).
- **Node.js**: Node.js 18+ with **pnpm**
- **PowerShell**: PowerShell 7 (`pwsh`)
- **Protobuf Compiler**: `protoc` (with `protoc-gen-go` plugin)

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
  `.github/actions/setup-go-runtime` installs the matching prebuilt release asset (`.tar.gz` on non-Windows runners, `.zip` on Windows), verifies its GitHub-provided SHA-256 digest, and sets `GOROOT`, `PATH`, and `GOTOOLCHAIN=local`.

### Testing & Verification
- **Run All Tests**:
  `go test ./...`
- **Run Package-Specific Tests**:
  `go test -v ./internal/...`

---

## 4. Strict AI Rules & Guidelines

1. **Mandatory Verification**: MUST run `go test ./...` or `pwsh ./build.ps1 c nb` to verify code changes before completion. NEVER claim success without empirical execution output.
2. **No Blind Assumptions**: ALWAYS inspect actual function signatures, types, and imports using file viewing tools before making edits.
3. **Error Handling & Quality**: NEVER swallow errors silently, return dummy fallbacks, or delete failing test assertions.
4. **Performance Safety**: Avoid unnecessary heap allocations or memory copies in hot execution paths (e.g., streaming, storage, hashing).
5. **Keep AGENTS.md Updated**: If your task alters build scripts, dependencies, project paths, or execution workflows, **you MUST update `AGENTS.md` before finishing**.
