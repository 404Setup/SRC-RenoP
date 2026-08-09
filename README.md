<p align="center">
  <img src="assets/banner.svg" alt="RenoP" width="720">
</p>

# RenoP

RenoP is a lightweight, rapidly deployable self-hosted Maven server for individuals and teams.

If you intend to use it for public hosting, RenoP currently does not support that.

## Features

- Hosted release, snapshot, and private repositories
- Maven mirror proxying with local caching and negative caching
- Browser, upload, user, token, and repository management in the web UI
- Local disk or S3-compatible object storage
- Repository permissions, Basic authentication, and upload tokens
- Embedded Javadoc browsing, checksums, health data, and runtime statistics

## Quick start

Download a release archive, extract it, and run `renop.exe` on Windows or
`./renop` on Unix-like systems. RenoP listens on `0.0.0.0:3000` by default.

On first start, RenoP creates an `admin` account. Set its password before starting the server:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='replace-this-password' ./renop
```

If the variable is not set, a random password is printed to the server log. Open `http://localhost:3000` after startup.

The default repository endpoints are:

```text
http://localhost:3000/releases
http://localhost:3000/snapshots
http://localhost:3000/private
```

Use one of these URLs in Maven's `<repositories>` or
`<distributionManagement>` configuration.

HTTP API reference (auth, tokens, Maven helpers, settings, updater, storage paths)
lives under [`docs/api/`](https://renop.pkg.one/docs/api/README). OpenAPI 3: [
`openapi.yaml`](https://renop.pkg.one/assets/openapi.yaml).

## Configuration

RenoP creates its configuration and state files in the working directory. Their paths can be overridden with environment
variables.

| Variable                       | Default             | Purpose                                                       |
|--------------------------------|---------------------|---------------------------------------------------------------|
| `RENOP_CONFIG`                 | `config.yaml`       | Server, frontend, and storage settings                        |
| `RENOP_REPOSITORIES`           | `repositories.yaml` | Repositories, mirrors, and per-repository S3 settings         |
| `RENOP_TOKENS`                 | `tokens.yaml`       | Accounts and access tokens                                    |
| `RENOP_INDEX`                  | `index.json`        | Persisted artifact index                                      |
| `RENOP_SESSIONS`               | `sessions.bin`      | Persisted login sessions (legacy `sessions.json` is migrated) |
| `RENOP_DEFAULT_ADMIN_PASSWORD` | generated           | Password used when the first admin account is created         |

Most settings can also be changed from the management UI. Restart the server after changing listener or TLS settings.

## Building

RenoP must be built with [our Go fork](https://github.com/404Setup/go/releases). The official Go toolchain will not work
because RenoP uses our changes to the runtime and standard library. You will also need PowerShell 7 and Node.js.

1. Check the `go` version in [`go.mod`](go.mod).
2. From the Go fork's releases, download the newest tag beginning with `go<version>` for your OS and architecture.
3. Check the download against `SHA256SUMS` from the same release.
4. Extract it, set `GOROOT` to the extracted `go` directory, add `GOROOT/bin` to `PATH`, and run `go version`.

```powershell
pwsh ./build.ps1
pwsh ./build.ps1 s
pwsh ./build.ps1 c
pwsh ./build.ps1 c nb
pwsh ./build.ps1 -Version v1.2.3 -Development false
```

```powershell
protoc -I proto --go_out=. --go_opt=module=renop proto/api/v1/api.proto
pnpm install --frozen-lockfile
pnpm --filter renop-html build
go generate ./frontend
```

Run the test suite with `go test ./...` (after the frontend dist has been generated).

## Price and Contribution

RenoP is completely free for all users, whether individuals, teams, or businesses and non-profit organizations of any
size.

RenoP guarantees no hidden data collection, no ads, no annoying pop-ups, no messy subscription tiers...

RenoP only has this one free community edition. If you'd like to contribute to RenoP, you can consider:

- Helping us find issues (submit valid Issues)
- Helping us fix issues (submit valid PRs)
- Helping us solve issues (support us via Patreon)
- Giving us motivation to solve issues (give us a Star)

If you want to maintain your own fork, you need to consider the following:

- Whether you can maintain it long-term
- Whether you can maintain a reasonable level of stability

Please be responsible for your own code, and also responsible for the community. Don't create a fork based solely on
impulse or a fleeting interest.

A better choice is to open an Issue or submit a PR — we do our best to listen to everyone's voice.

## License

RenoP is licensed under the [Mozilla Public License 2.0](LICENSE) and is marked as incompatible with secondary licenses.

Third-party components retain their own licenses. Copyright notices, SPDX identifiers, Apache `NOTICE` excerpts, and
full license texts are in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) (also shipped in release archives). RenoP
used [Reposilite](https://github.com/dzikoysk/reposilite) as a design reference only; its code is not redistributed
here.
