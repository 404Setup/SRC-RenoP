---
title: Cargo (Rust) Registry
order: 2
category: Guides
description: Creating a Cargo repository, configuring Sparse Index, publishing, ownership, and Cargodoc
---

# Cargo (Rust) Registry Guide

Create a repository with format `cargo` before configuring clients. The examples use repository name `crates`. RenoP
implements the Cargo Sparse Index protocol and streams crate archives without a Git index clone.

## Configure Cargo (`.cargo/config.toml`)

```toml
[registries.renop]
index = "sparse+http://localhost:3000/crates/"

# Optional: replace default crates.io upstream
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/crates/"
```

Use HTTPS in production. The repository's `config.json` advertises download and API routes. A private repository sets
`auth-required` and requires credentials for index and crate reads.

## Authentication

Create a dedicated expiring API Token. Typical scopes are `repository:read`, `repository:publish`, and `package:create`
for first publication. Add `package:lifecycle` for archive/yank actions or `team:manage` for owner administration.

```bash
cargo login --registry renop
# Paste your RenoP token when prompted
```

Cargo stores the value in `~/.cargo/credentials.toml`:

```toml
[registries.renop]
token = "your_renop_token"
```

The Token is sent as the complete `Authorization` value. RenoP still intersects its scopes and target restrictions with
the account's current repository and package-team permissions.

## Dependencies and publication

### Add a dependency (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### Publish a crate

```bash
cargo publish --registry renop
```

The first successful publication reserves the normalized name and grants the publisher L4. RenoP rejects a name that
exists locally or on an applicable enabled mirror. If the upstream check is inconclusive, publication fails safely with
`503` and does not reserve the package. Later versions require the package team's publication level.

### Search, yank, and unyank

```bash
# Search crates
cargo search --registry renop my-crate

# Yank a version
cargo yank --registry renop --version 0.1.0 my-crate

# Unyank
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

Package owners manage L0-L4 collaborators and invitations from the package page. Mirrored crates are marked as upstream
content, have no local owner, and remain pull-only until a distinct available name is published locally.

## Cargodoc

When documentation is uploaded, RenoP validates and extracts rustdoc into a sandboxed viewer. Enable Cargodoc and set
its size limits in `config.yaml`.

Access URL: `http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
