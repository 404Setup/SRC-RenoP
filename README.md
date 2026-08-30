<p align="center">
  <img src="assets/banner.svg" alt="RenoP" width="720">
</p>

# RenoP

RenoP is a lightweight, self-hosted artifact repository server for individuals and small teams. It ships as a single
binary with an embedded web UI and supports multiple registry protocols out of the box.

## Table of Contents

- [Features](#features)
- [Supported Registry Protocols](#supported-registry-protocols)
- [Quick Start](#quick-start)
- [Authentication](#authentication)
- [Configuration](#configuration)
    - [Environment Variables](#environment-variables)
    - [config.yaml Reference](#configyaml-reference)
    - [repositories.yaml Reference](#repositoriesyaml-reference)
- [Repository Types](#repository-types)
- [Mirror Proxying](#mirror-proxying)
- [Storage Backends](#storage-backends)
- [GPG Signature Verification](#gpg-signature-verification)
- [Outbound Proxy](#outbound-proxy)
- [Global Teams](#global-teams)
- [Message Center](#message-center)
- [Audit Log](#audit-log)
- [System Service Installation](#system-service-installation)
- [Building from Source](#building-from-source)
- [HTTP API Reference](#http-api-reference)
- [Price and Contribution](#price-and-contribution)
- [License](#license)

---

## Features

- **Maven repository** — Release, snapshot, and private repositories with Maven 2 layout
- **Cargo (Rust) registry** — Sparse index protocol, crate ownership, yank/unyank, and documentation uploads
- **npm registry** — Explicit package reservation, immutable versions, private scopes, dist-tags, teams, and mirrors
- **Docker / OCI registry** — Full OCI Distribution Specification v1.1.0 with Bearer token authentication
- **Mirror proxying** — Upstream proxy with local caching, negative caching, per-artifact allowlists, and TTL
- **Multi-backend storage** — Local disk or any S3-compatible object store (per-repository)
- **GPG signature verification** — Enforce detached `.asc` signatures before accepting Maven artifacts
- **Web management UI** — Browser, upload, user management, token management, and repository settings
- **FIDO2 / WebAuthn** — Passwordless login and MFA with hardware security keys
- **In-app message center** — Durable per-user inbox with admin broadcast notifications
- **Activity audit log** — Immutable per-user action history with manager visibility
- **Global teams** — Immutable shared prefixes, T1-T4 collaboration, invitations, and configurable account limits
- **Javadoc and Cargo-doc preview** — In-browser viewing of extracted documentation jars and Cargo doc tarballs
- **Embedded SVG badges** — Latest-version badges for Maven artifacts
- **Online and offline updater** — One-click or automated binary updates without external tools
- **System service integration** — `--install` and `--uninstall` support for Windows Services, systemd, OpenRC, macOS
  LaunchDaemons, and BSD rc.d
- **Debug profiling** — Optional pprof heap, allocation, and goroutine dumps behind an authenticated endpoint

---

## Supported Registry Protocols

| Protocol              | URL prefix          | Specification                                                                       |
|-----------------------|---------------------|-------------------------------------------------------------------------------------|
| Maven 2               | `/{repo}/`          | Maven repository layout                                                             |
| Cargo sparse index    | `/{repo}/`          | [RFC 3239](https://rust-lang.github.io/rfcs/3239-cargo-sparse-registry.html)        |
| npm registry          | `/{repo}/`          | npm-compatible packuments, publication, tarballs, dist-tags, and search           |
| Docker / OCI Registry | `/v2/`              | [OCI Distribution Spec v1.1.0](https://github.com/opencontainers/distribution-spec) |
| Javadoc preview       | `/javadoc/{repo}/`  | RenoP extension                                                                     |
| Cargo-doc preview     | `/cargodoc/{repo}/` | RenoP extension                                                                     |

---

## Quick Start

Download a release archive, extract it, and run the binary.

```bash
# Unix-like systems
./renop

# Windows
renop.exe
```

RenoP listens on `0.0.0.0:3000` by default.

**Set the admin password before first start:**

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='your-secure-password' ./renop
```

If the variable is not set, a randomly generated password is printed to the server log on the very first startup. Open
`http://localhost:3000` to access the web UI.

**Default Maven repository endpoints:**

```text
http://localhost:3000/releases
http://localhost:3000/snapshots
http://localhost:3000/private
```

Use one of these URLs in Maven's `<repositories>` or `<distributionManagement>` configuration.

**Cargo sparse registry:**

```toml
# .cargo/config.toml
[registries.my-registry]
index = "sparse+http://localhost:3000/rust-id/"
credential-provider = "cargo:token"
```

**Docker registry:**

```bash
docker login localhost:3000
docker pull localhost:3000/my-image:latest
docker push localhost:3000/my-image:latest
```

**npm registry:**

```ini
registry=http://localhost:3000/javascript/
//localhost:3000/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

---

## Authentication

RenoP separates browser sessions from machine credentials:

| Method                | Use case                    | Details                                                                                                     |
|-----------------------|-----------------------------|-------------------------------------------------------------------------------------------------------------|
| **Session cookie**    | Browser / web UI            | Set after login as the HttpOnly `renop_session` cookie. Session secrets are not accepted in headers or URLs. |
| **Bearer API token**  | API and CI/CD automation   | `Authorization: Bearer <rnp_pat_...>` with endpoint scopes capped by the account's current permissions.      |
| **Basic auth**        | Package clients and CI/CD  | `username:password` or `username:API-token`; accepted only by standard package protocols and upload flows.   |
| **Docker Bearer JWT** | Docker / OCI registry      | Short-lived token issued by `/v2/token` with pull/push actions limited by API-token and package permissions. |

**FIDO2 / WebAuthn** is also supported as a passwordless login mechanism. Register a hardware key from the web UI
profile page.

### Permissions

Every account carries a set of permission strings. Built-in shortcuts:

| Permission         | Meaning                               |
|--------------------|---------------------------------------|
| `manager`          | Full administrative access            |
| `canview:*`        | Read access to all repositories       |
| `canupdate:*`      | Write access to all repositories      |
| `canview:<repo>`   | Read access to a specific repository  |
| `canupdate:<repo>` | Write access to a specific repository |

API tokens are named, optionally expiring 256-bit credentials. Their fine-grained scopes are always intersected with
the owning account's live roles, repository permissions, and package-team membership. Secrets are displayed once,
stored only as SHA-256 lookup digests, and revoked immediately. Legacy plaintext upload tokens migrate automatically
to `repository:read` and `repository:publish` scopes.

---

## Configuration

### Environment Variables

RenoP reads its configuration and state file locations from environment variables. Paths are relative to the working
directory unless they are absolute.

| Variable                       | Default             | Purpose                                                                       |
|--------------------------------|---------------------|-------------------------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Server, frontend, storage, database, updater, proxy, and audit settings       |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repository definitions, mirrors, and per-repository S3 settings               |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Legacy account import source; plaintext upload tokens are migrated to the database |
| `RENOP_INDEX`                  | `index.json`        | Persisted Maven artifact index (rebuilt on startup or on demand)              |
| `RENOP_SESSIONS`               | `sessions.bin`      | Persisted browser sessions (legacy `sessions.json` is migrated automatically) |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | *(generated)*       | Initial password for the `admin` account on first startup                     |

Most settings can be changed live from the management UI without restarting the server. Changes to the listener address,
TLS certificates, or debug mode require a restart.

### `config.yaml` Reference

```yaml
# Root-level storage settings (also the StorageConfig domain in the settings API)
storage_path: storage             # Base directory for on-disk artifact storage
enable_javadoc_preview: true      # Enable in-browser Javadoc and Cargo-doc preview
javadoc_extract_path: ""          # Override extraction directory (defaults to storage_path/.javadoc)
max_javadoc_size_mb: 48           # Maximum size (MiB) of a documentation archive accepted for extraction

frontend:
  id: my-repo                     # Internal instance identifier
  title: My Repository            # Browser tab and UI title
  description: ""                 # Short description shown on the landing page
  organization_website: ""        # URL linked from the organization logo
  organization_logo: /svg/logo.svg
  background_url: ""              # Public WebP <= 5 MiB; served as a login-page background
  font_preset: system             # system | inter | noto_sans | open_sans | source_sans | custom
  font_url: ""                    # Direct font file or Google Fonts CSS URL when font_preset is custom
  icp_license: ""                 # Optional: ICP filing number (displayed in footer, China)
  public_security_filing: ""      # Optional: Public security filing number (China)
  legal_notice_url: ""            # Optional: Absolute URL for the legal notice footer link

server:
  host: 0.0.0.0
  port: 3000
  ssl_enabled: false
  ssl_cert_path: ""
  ssl_key_path: ""
  domains: # Canonical public hostnames; used for CORS and cookie domain
    - localhost
  cors_origins: [ ]                # Additional CORS origins (supports *.example.com wildcards and *)
  trusted_proxies: [ ]             # Trusted reverse-proxy IP ranges for X-Forwarded-For
  cdn_ip_header: X-Forwarded-For  # Header used to extract the real client IP
  file_cache_size_mb: 16          # In-memory LRU cache for hot artifact reads (0 = disabled)
  max_active_requests: 2000       # Concurrency limit; excess requests receive 429
  enable_compression: false       # Brotli/gzip response compression
  debug_mode: false               # Enable pprof dump endpoints (restart required)
  gpg:
    key_servers: # HTTPS-only key servers for GPG key resolution
      - https://keyserver.ubuntu.com
      - https://keys.openpgp.org
      - https://pgp.mit.edu
  audit_log:
    retention_days: 14            # Delete entries older than this many days
    max_rows: 10000               # Delete oldest entries when total exceeds this count

super_teams:
  create_limit: 5                 # Default maximum teams created by one account
  join_limit: 20                  # Default maximum memberships, including owned teams

updater:
  channel: release                # release | nightly
  mode: manual                    # manual | auto_check | auto_install | safe_install

database:
  driver: sqlite3                 # sqlite3 | mysql | postgres | clickhouse
  dsn: renop.db                   # SQLite path or a standard MySQL/PostgreSQL/ClickHouse DSN
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime_sec: 300

proxy:
  selected: ""                    # Name of the active outbound proxy (empty = direct)
  proxies:
    - name: corp-proxy
      url: http://proxy.example.com:8080   # HTTP, HTTPS, or SOCKS5
      username: ""
      password: ""
```

### `repositories.yaml` Reference

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC            # PUBLIC | HIDDEN | PRIVATE
    allow_redeployment: true      # Allow overwriting an existing artifact version
    require_gpg_signature: false  # Require detached .asc signatures for Maven artifacts
    mirrors: # Zero or more upstream mirrors
      - name: central
        url: https://repo1.maven.org/maven2
        persist: true             # Cache downloaded artifacts to local storage
        cache_ttl_secs: 3600      # How long a cached miss is considered valid
        negative_cache: true      # Cache 404 responses to avoid repeated upstream requests
        timeout_secs: 30          # HTTP request timeout
        proxy: ""                 # Named proxy override (empty = global proxy; "direct" = bypass)
        authorization: null       # null | {method: basic, login: user, password: pass}
        allow_artifacts: [ ]       # Group ID prefix allowlist (empty = all)
        deny_artifacts: [ ]        # Group ID prefix denylist
    s3:
      enabled: false
      endpoint: ""                # S3-compatible endpoint URL
      bucket: ""
      key_prefix: ""              # Optional object key prefix within the bucket
      region: auto
      access_key_id: ""
      secret_access_key: ""
      force_path_style: true      # Required for MinIO and most non-AWS S3 providers
      redirect_downloads: false   # Issue signed redirects instead of proxying blob downloads

  rust-id:
    name: rust-id
    format: cargo                 # Mark this repository as a Cargo sparse registry
    visibility: PUBLIC
    mirrors: [ ]
    s3:
      enabled: false
      # ... (same S3 fields as above)

  javascript:
    name: javascript
    format: npm                   # npm-compatible package registry
    visibility: PUBLIC
    mirrors: [ ]
```

**Reserved repository names** (cannot be used as repository names): `css`, `js`, `svg`, `api`, `javadocs`,
`assets`, `cargodoc`, `v2`.

---

## Repository Types

### Maven Repositories

Maven repositories follow the standard Maven 2 directory layout (`groupId/artifactId/version/artifactId-version.jar`).
RenoP supports:

- Release, snapshot, and private visibility modes
- Automatic `maven-metadata.xml` generation and update on upload
- Embedded Javadoc preview (from `.jar` files containing a Javadoc directory)
- Checksum generation (`md5`, `sha1`, `sha256`, `sha512`)
- GPG signature enforcement via detached `.asc` files

### Cargo (Rust) Repositories

Cargo repositories implement
the [sparse index protocol](https://rust-lang.github.io/rfcs/3239-cargo-sparse-registry.html). Set `format: cargo` in
`repositories.yaml` and configure Cargo clients with:

```toml
# .cargo/config.toml
[registries.my-registry]
index = "sparse+http://localhost:3000/<repo-name>/"
credential-provider = "cargo:token"
```

Cargo-specific operations supported:

- Publish new crate versions (`cargo publish`)
- Yank and unyank specific versions (`cargo yank`, `cargo yank --undo`)
- Per-crate ownership management with invitation-based workflows
- Cargo-doc tarball upload and sandboxed in-browser viewing at `/cargodoc/{repo}/{crate}/{version}/`
- Crate search and version listing

### npm Repositories

Create an `npm` repository and reserve each local package from the web interface before publishing. RenoP validates
the streamed tarball, stores immutable semantic versions, serves full or abbreviated packuments, and supports standard
dist-tag, deprecation, unpublish, search, and audit requests. Scoped packages may be private; L0-L4 package teams control
read, publish, lifecycle, team, and ownership operations. Upstream mirrors remain pull-only.

```bash
npm config set registry http://localhost:3000/javascript/
npm publish
npm install @example/library
```

### Docker / OCI Registry

RenoP implements the [OCI Distribution Specification v1.1.0](https://github.com/opencontainers/distribution-spec)
at `/v2/`. Authentication uses the Bearer token challenge flow.

```bash
docker login localhost:3000
docker pull localhost:3000/my-namespace/my-image:tag
docker push localhost:3000/my-namespace/my-image:tag
```

---

## Mirror Proxying

Each repository can define one or more upstream mirrors. When a requested artifact is not found locally, RenoP fetches
it from the mirrors in order and optionally caches the result.

**Cache behavior:**

- Positive hits (artifact found upstream) are cached on disk if `persist: true`.
- Negative hits (upstream returns 404) are cached for `cache_ttl_secs` when `negative_cache: true`.
- Cached files are served on subsequent requests without contacting the upstream.

**Artifact filtering:**

- `allow_artifacts` is a list of Maven group ID prefixes. Only artifacts whose group ID starts with one of these values
  are fetched from this mirror. An empty list allows all group IDs.
- `deny_artifacts` blocks specific prefixes regardless of the allowlist.

**Per-mirror proxy routing:**

| Value      | Behavior                                   |
|------------|--------------------------------------------|
| `""`       | Follow the global `proxy.selected` setting |
| `"direct"` | Bypass the global proxy for this mirror    |
| `"<name>"` | Use a named proxy from `proxy.proxies`     |

---

## Storage Backends

### Local Disk (default)

Artifacts are stored under `storage_path` using the same Maven layout as the repository URL:

```
{storage_path}/{repo_name}/{groupId-with-slashes}/{artifactId}/{version}/{file}
```

### S3-Compatible Object Storage

Set `s3.enabled: true` in a repository definition to store new uploads in an S3 bucket. Existing files on local disk
continue to be served from disk. Set `redirect_downloads: true` to issue pre-signed URLs to clients instead of proxying
blob downloads through RenoP.

Compatible providers: AWS S3, MinIO, Cloudflare R2, Wasabi, Backblaze B2, and any S3-compatible provider.

---

## GPG Signature Verification

When `require_gpg_signature: true` is set on a repository, RenoP requires a detached OpenPGP signature file (`.asc`) to
accompany every uploaded Maven artifact. Artifacts submitted without a valid, matching signature are quarantined under
`.renop.tmp.gpg/` and the upload is rejected.

Key lookup uses the HTTPS key servers listed under `server.gpg.key_servers`. Only HTTPS origins are accepted; embedded
paths, credentials, query strings, or fragments in key server URLs are rejected at configuration time.

---

## Outbound Proxy

RenoP can route upstream mirror traffic through an HTTP, HTTPS, or SOCKS5 proxy. Define one or more named proxies under
`proxy.proxies` and set `proxy.selected` to the name of the globally active one. Individual mirrors can override this on
a per-mirror basis using the `proxy` field.

---

## Global Teams

Global teams are instance-wide collaboration identities with immutable 2–64 character prefixes. Membership is stored
against immutable account identities, while the UI and API expose usernames. T1 provides read access, T2 maps to
publication and version maintenance, T3 manages members, and T4 owns team configuration. T3 can manage T1/T2 members;
only T4 or a system administrator can grant or manage T3/T4.

The account menu opens `/account/teams`. Creation and membership limits default to `super_teams.create_limit` and
`super_teams.join_limit`; managers can set account-specific overrides through
`PUT /api/super-teams/users/{username}/limits`. A value of `-1` restores inheritance and zero prevents the corresponding
operation. Invitations are one-time message-center actions and expire after seven days.

---

## Message Center

RenoP includes a durable per-user inbox. Messages are persisted in the database and survive server restarts. Managers
can send broadcast announcements to all users or to a specific list of recipients.

**Supported message kinds:**

| Kind           | Description                                           |
|----------------|-------------------------------------------------------|
| `announcement` | Administrator broadcast to one or more users          |
| `team_invite`  | Cargo crate ownership invitation requiring a response |
| `system`       | Internal server event                                 |

Messages with a pending action (e.g., a team invitation awaiting acceptance or decline) cannot be deleted until the
action is resolved.

Endpoints under `GET/POST/DELETE /api/messages/...` require an authenticated session. Admin endpoints
(`GET /api/messages/admin/users`, `POST /api/messages/admin`) require the `manager` permission.

---

## Audit Log

Every significant API action is recorded in the audit log with the following fields: acting username, operator (which
may differ when an admin acts on behalf of another user), authentication method, session ID, client IP, a
machine-readable action code, and a human-readable description.

- Users can view their own log at `GET /api/profile/audit-logs`.
- Managers can view and clear any user's log at `GET /api/users/{username}/audit-logs`.
- Users cannot delete their own log entries (`DELETE /api/profile/audit-logs` always returns `403`).

Retention is controlled by `server.audit_log.retention_days` and `server.audit_log.max_rows`.

---

## System Service Installation

RenoP can register itself as a platform service to start automatically at boot.

```bash
# Run as administrator / root
./renop --install

# Configure a local Caddy reverse proxy and synchronize config.yaml
./renop --install-caddy --hostname renop.example.com

# Remove the service registration
./renop --uninstall
```

The Caddy command discovers common `Caddyfile` locations, validates the generated site with Caddy, replaces each file
atomically, rolls both back if reload fails, and reloads Caddy. It changes RenoP to listen on `127.0.0.1`, disables
RenoP TLS, and adds the hostname to `server.domains`; restart RenoP afterward. Use `--caddyfile`, `--config`, or
`--caddy-binary` for nonstandard paths.
`--skip-reload` is available for offline preparation when Caddy is not installed on the current machine.

| Platform           | Service backend                       |
|--------------------|---------------------------------------|
| Windows            | Windows Service Control Manager (SCM) |
| Linux with systemd | `systemd` unit file                   |
| Linux with OpenRC  | OpenRC init script                    |
| macOS              | LaunchDaemons plist                   |
| BSD                | `rc.d` script                         |

---

## Building from Source

RenoP must be built with the [custom Go fork](https://github.com/404Setup/go/releases). The standard Go toolchain will
not work because RenoP relies on runtime and standard library modifications from that fork. You also need PowerShell 7
and Node.js 18+.

**Toolchain setup:**

1. Check the required Go version in [`go.mod`](go.mod).
2. Download the newest release tag beginning with `go<version>` for your OS and architecture from the fork's GitHub
   releases page.
3. Verify the download against `SHA256SUMS` in the same release.
4. Extract the archive, set `GOROOT` to the extracted `go/` directory, and add `GOROOT/bin` to `PATH`.
5. Confirm with: `go version`

**Build commands:**

```powershell
# Full matrix release build (all OS/arch combinations, produces raw .br packages)
pwsh ./build.ps1

# Brotli-packaged release build for the current OS/arch only
pwsh ./build.ps1 c

# Local development build (unzipped binary, no frontend rebuild)
pwsh ./build.ps1 c nb

# Cross-compile for a single OS only
pwsh ./build.ps1 s

# Release build with explicit version string
pwsh ./build.ps1 -Version v1.2.3 -Development false
```

**Frontend and protobuf generation:**

```powershell
pnpm install --frozen-lockfile
pnpm --filter renop-html build
go generate ./frontend

# Regenerate protobuf Go bindings
protoc -I proto --go_out=. --go_opt=module=renop proto/api/v1/api.proto
```

**Run the test suite** (requires the frontend `dist/` to be present):

```bash
go test ./...
```

---

## HTTP API Reference

The management API is mounted at `/api`. Maven artifact storage is served directly at `/{repo}/{path}`. The Docker/OCI
registry is at `/v2/`. Cargo endpoints are sub-paths of the repository URL.

Most management endpoints use `application/x-protobuf` (Protocol Buffers) for request and response bodies. Schema
definitions are in [`proto/api/v1/api.proto`](proto/api/v1/api.proto). Error responses are generally plain text; always
prefer the HTTP status code over the body for programmatic handling.

- **OpenAPI 3.0.3 specification:** [`openapi.yaml`](https://renop.pkg.one/assets/openapi.yaml)
- **Hosted API documentation:** [`docs/api/README`](https://renop.pkg.one/docs/api/README)

### Endpoint Groups

| Tag        | Routes                                                                              | Notes                               |
|------------|-------------------------------------------------------------------------------------|-------------------------------------|
| `auth`     | `POST /api/auth/login`, `/api/auth/logout`, `/api/auth/me`, `/api/auth/profile/...` | Cookie sessions, API tokens, and FIDO2 |
| `tokens`   | `GET/PUT/DELETE /api/tokens/{name}`, `/api/tokens/{name}/sessions/...`              | Account CRUD (manager-only)         |
| `maven`    | `GET /api/maven/details/...`, `/api/maven/versions/...`, `/api/maven/latest/...`    | File metadata and version helpers   |
| `storage`  | `GET/HEAD/PUT/POST/DELETE /{repo}/{path}`                                           | Raw artifact access (Maven layout)  |
| `cargo`    | `/{repo}/api/v1/...`, `/{repo}/config.json`                                         | Cargo sparse registry               |
| `docker`   | `/v2/...`                                                                           | OCI Distribution Spec v1.1.0        |
| `javadoc`  | `/javadoc/{repo}/{path}`                                                            | Javadoc jar preview                 |
| `status`   | `GET /api/status/health`, `/api/status/instance`, `/api/status/snapshots`           | Health and metrics                  |
| `settings` | `GET/PUT /api/settings/domain/{name}`, `/api/settings/maven/repositories/...`       | Configuration management            |
| `updater`  | `POST /api/updater/check`, `/api/updater/install`, `/api/updater/restart`           | Update management                   |
| `messages` | `GET/POST/DELETE /api/messages/...`                                                 | User inbox and admin broadcast      |
| `super-teams` | `GET/POST/PUT/DELETE /api/super-teams/...`                                      | Global teams, roles, invites, limits |
| `audit`    | `GET /api/profile/audit-logs`, `GET/DELETE /api/users/{username}/audit-logs`        | Activity log                        |
| `debug`    | `GET /api/debug/memory/...`                                                         | pprof dumps (requires `debug_mode`) |

---

## Price and Contribution

RenoP is completely free for all users — individuals, teams, businesses, and non-profit organizations of any size.

RenoP guarantees no hidden data collection, no advertisements, no annoying pop-ups, and no subscription tiers. RenoP has
only this one free community edition.

If you would like to contribute:

- **Report issues** — Submit valid bug reports via GitHub Issues
- **Fix issues** — Submit pull requests with tests
- **Sponsor** — Support ongoing development via Patreon
- **Star** — Give the repository a star on GitHub

If you intend to maintain a long-term fork, please consider whether you can sustain it and maintain a reasonable
stability standard for your users. Opening an issue or submitting a pull request is almost always a better alternative.

---

## License

RenoP is licensed under the [Mozilla Public License 2.0](LICENSE) and is marked as incompatible with secondary licenses.

Third-party components retain their own licenses. Copyright notices, SPDX identifiers, Apache `NOTICE` excerpts, and
full license texts are in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) (also shipped in release archives). RenoP
Earlier design exploration referenced [Reposilite](https://github.com/dzikoysk/reposilite). RenoP is an independent,
integrated Central-style publication platform rather than a Reposilite derivative; Reposilite code is not redistributed.
