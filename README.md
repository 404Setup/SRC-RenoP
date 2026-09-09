<p align="center">
  <img src="assets/banner.svg" alt="RenoP" width="720">
</p>

# RenoP

A self-hosted package repository for Maven, Cargo, npm, Docker/OCI, and generic files, with a web management interface
embedded in one executable.

[Website](https://renop.mvnc.one/) ·
[Documentation](https://renop.mvnc.one/docs) ·
[Downloads](https://renop.mvnc.one/download) ·
[Releases](https://github.com/404Setup/SRC-RenoP/releases)

## Features

- Local disk or S3-compatible storage; upstream mirrors with per-mirror proxy routing.
- SQLite, PostgreSQL, MySQL, and native ClickHouse databases.
- Password, Passkey, GitHub OAuth, and scoped API tokens.
- Package and global teams, publication reviews, ownership transfers, and quotas.
- GPG verification, audit logs, messages, download statistics, and online updates.
- System service management on Windows, Linux, macOS, and BSD, plus Caddy integration.

## Quick start

[Download a build](https://renop.mvnc.one/download) for your platform and extract it to `renop` or `renop.exe`.
The download page supports ZIP conversion for raw Brotli packages.

Linux / macOS:

```bash
chmod +x ./renop
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

Windows PowerShell:

```powershell
$env:RENOP_DEFAULT_ADMIN_PASSWORD = 'replace-this-password'
.\renop.exe
```

Open `http://localhost:3000` and sign in as `admin`. If the password variable is omitted, the initial password is
generated and printed once to the server log. Configuration is created on first start; manage repositories and
settings through the web interface.

The default listener is `0.0.0.0:3000`. Follow the
[production checklist](https://renop.mvnc.one/docs/deployment/production-checklist) before exposing the instance.
See [Quickstart](https://renop.mvnc.one/docs/getting-started/quickstart) for repository setup.

## Documentation

| Topic           | Guides                                                                                                                                                                                                                                         |
|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Package clients | [Maven](https://renop.mvnc.one/docs/guides/maven-client), [Cargo](https://renop.mvnc.one/docs/guides/cargo-registry), [npm](https://renop.mvnc.one/docs/guides/npm-registry), [Docker/OCI](https://renop.mvnc.one/docs/guides/docker-registry) |
| Configuration   | [Overview](https://renop.mvnc.one/docs/configuration/overview), [repositories](https://renop.mvnc.one/docs/configuration/repositories), [databases](https://renop.mvnc.one/docs/configuration/database)                                        |
| Deployment      | [Services](https://renop.mvnc.one/docs/deployment/daemon), [reverse proxy](https://renop.mvnc.one/docs/deployment/reverse-proxy), [backup and recovery](https://renop.mvnc.one/docs/deployment/backup-and-recovery)                            |
| Security        | [Security model](https://renop.mvnc.one/docs/security/security-model), [tokens and keys](https://renop.mvnc.one/docs/security/tokens-and-keys)                                                                                                 |
| API             | [Reference](https://renop.mvnc.one/docs/api/README), [OpenAPI](https://renop.mvnc.one/assets/openapi.yaml)                                                                                                                                     |
| Support         | [Troubleshooting](https://renop.mvnc.one/docs/guides/troubleshooting), [issues](https://github.com/404Setup/SRC-RenoP/issues)                                                                                                                  |

## Building from source

Use the [404Setup Go fork](https://github.com/404Setup/go/releases) matching [go.mod](go.mod), PowerShell 7, Node.js
24+,
the pnpm version pinned in [package.json](package.json), `protoc`, and `protoc-gen-go`.

```powershell
pnpm install --frozen-lockfile
pwsh ./build.ps1 c nb
```

This generates protobuf bindings, builds the embedded frontend, and compiles an unpackaged binary for the current
platform. Use `pwsh ./build.ps1 c` for a raw Brotli release package or `pwsh ./build.ps1` for all targets.
See [AGENTS.md](AGENTS.md) for contribution and verification guidance.

## License

[Mozilla Public License 2.0](LICENSE), incompatible with secondary licenses.
See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for dependency licenses.
