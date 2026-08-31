<p align="center">
  <img src="assets/banner.svg" alt="RenoP" width="720">
</p>

# RenoP

RenoP is a self-hosted package repository for teams and controlled internal services. One executable serves the web
management interface together with native Maven, Cargo, npm, Docker/OCI, and unstructured-file endpoints.

[Documentation](web/content/docs/en-US/getting-started/introduction.md) ·
[Quick start](web/content/docs/en-US/getting-started/quickstart.md) ·
[API reference](web/content/docs/en-US/api/README.md) ·
[Releases](https://github.com/404Setup/SRC-RenoP/releases)

## Highlights

- Maven, Cargo sparse registry, npm, Docker/OCI, and generic file repositories
- Local disk or per-repository S3-compatible storage
- Upstream mirrors with bounded caching and per-mirror proxy routing
- Password, Passkey, GitHub OAuth, browser session, and scoped API-token authentication
- Package teams, global T1-T4 teams, publication quotas, moderation, and ownership review
- GPG verification, audit history, durable messages, statistics, and online updates
- SQLite, PostgreSQL, MySQL, and native ClickHouse database drivers
- Windows Services, systemd, OpenRC, macOS LaunchDaemons, BSD rc.d, and Caddy integration

## Registry endpoints

| Protocol              | URL prefix          | Guide |
|-----------------------|---------------------|-------|
| Maven 2               | `/{repo}/`          | [Maven client](web/content/docs/en-US/guides/maven-client.md) |
| Cargo sparse registry | `/{repo}/`          | [Cargo registry](web/content/docs/en-US/guides/cargo-registry.md) |
| npm registry          | `/{repo}/`          | [npm registry](web/content/docs/en-US/guides/npm-registry.md) |
| Docker / OCI          | `/v2/`              | [Docker registry](web/content/docs/en-US/guides/docker-registry.md) |
| Javadoc preview       | `/javadoc/{repo}/`  | [Storage API](web/content/docs/en-US/api/storage.md) |
| Cargo-doc preview     | `/cargodoc/{repo}/` | [Cargo API](web/content/docs/en-US/api/cargo.md) |

## Quick start

Download and extract a release, then set the initial administrator password before the first start.

Unix-like systems:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Windows PowerShell:

```powershell
$env:RENOP_DEFAULT_ADMIN_PASSWORD = 'replace-this-password'
.\renop.exe
```

RenoP listens on `0.0.0.0:3000` by default. Open `http://localhost:3000`; if the password variable was omitted, use the
random initial password printed once to the server log. The initial configuration includes the `releases`, `snapshots`,
and `private` Maven repositories. Create other repository formats from the management interface.

Before exposing an instance, follow the
[production checklist](web/content/docs/en-US/deployment/production-checklist.md), configure a
[trusted reverse proxy](web/content/docs/en-US/deployment/reverse-proxy.md), and test
[backup and recovery](web/content/docs/en-US/deployment/backup-and-recovery.md).

## Configuration

RenoP creates its configuration on first start. Paths may be overridden with environment variables:

| Variable                       | Default             | Purpose |
|--------------------------------|---------------------|---------|
| `RENOP_CONFIG`                 | `config.yaml`       | Server, database, frontend, proxy, and security settings |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repository formats, visibility, mirrors, and storage |
| `RENOP_INDEX`                  | `index.json`        | Persisted artifact index |
| `RENOP_SESSIONS`               | `sessions.bin`      | Persisted browser sessions |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | Generated once      | Password used only when creating the initial `admin` account |

Most settings are editable from the management interface. See the
[configuration overview](web/content/docs/en-US/configuration/overview.md),
[database guide](web/content/docs/en-US/configuration/database.md), and
[repository guide](web/content/docs/en-US/configuration/repositories.md) for the complete schema and operational
constraints.

## Authentication

- Browser operations use the HttpOnly session cookie.
- Automation uses one-time-displayed, scoped Bearer API tokens.
- Standard package clients may use Basic credentials where their protocols require them.
- Docker clients exchange accepted credentials for short-lived, action-scoped Bearer tokens.

Session secrets are not accepted in headers, URLs, or configuration files. Token scopes are intersected with the
account's current permissions and optional repository, package, team, or domain restrictions. See the
[security model](web/content/docs/en-US/security/security-model.md) and
[token guide](web/content/docs/en-US/security/tokens-and-keys.md).

## Service installation

Run service commands with the platform's required administrator privileges:

```bash
./renop --install
./renop --install-caddy --hostname renop.example.com
./renop --uninstall
```

See the [daemon](web/content/docs/en-US/deployment/daemon.md) and
[reverse-proxy](web/content/docs/en-US/deployment/reverse-proxy.md) guides for platform-specific behavior.

## Building from source

RenoP requires Go 1.28+ from the [404Setup Go fork](https://github.com/404Setup/go/releases), PowerShell 7, Node.js
18+, pnpm, `protoc`, and `protoc-gen-go`. The standard Go toolchain is not supported.

```powershell
pnpm install --frozen-lockfile
pwsh ./build.ps1 c nb
go test -count=1 -p 1 ./...
```

`build.ps1 c nb` regenerates both Go protobuf bindings and the embedded frontend, including ignored Brotli, Zstandard,
gzip, and deflate static-asset sidecars, before producing an unpackaged binary for the current platform. Other build
modes are:

| Command | Output |
|---------|--------|
| `pwsh ./build.ps1 c` | Current-platform raw Brotli release package |
| `pwsh ./build.ps1 s` | Mainstream target matrix |
| `pwsh ./build.ps1` | Full target matrix |

Generate Go bindings directly with:

```powershell
protoc -I proto --go_out=. --go_opt=module=renop proto/api/v1/api.proto proto/storage/v1/session.proto
```

Frontend and documentation checks are available through `pnpm run test:frontend`, `pnpm run check:i18n`, and
`pnpm run test:web`.

## API and operations documentation

- [Management API](web/content/docs/en-US/api/README.md) and
  [OpenAPI 3.0.3](https://renop.pkg.one/assets/openapi.yaml)
- [Architecture](web/content/docs/en-US/getting-started/architecture.md)
- [Publication reviews](web/content/docs/en-US/api/reviews.md) and
  [publication quotas](web/content/docs/en-US/api/publication-quotas.md)
- [Global teams](web/content/docs/en-US/api/global-teams.md)
- [S3 storage](web/content/docs/en-US/deployment/storage-s3.md)
- [Troubleshooting](web/content/docs/en-US/guides/troubleshooting.md)

## Contributing

Bug reports and focused pull requests with relevant tests are welcome through the GitHub repository.

## License

RenoP is licensed under the [Mozilla Public License 2.0](LICENSE) and is marked as incompatible with secondary licenses.
Third-party notices and license texts are maintained in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
