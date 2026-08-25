---
title: Cargo (Rust) Registry
order: 2
category: Guides
description: Configuring Cargo Sparse Index, publishing crates, and viewing Cargodoc documentation
---

# Cargo (Rust) Registry Guide

RenoP implements the modern Cargo Sparse Index protocol (default in Cargo 1.68+), allowing instant dependency resolution
without cloning monolithic git index repositories.

## 1. Configure Cargo (`.cargo/config.toml`)

Add the registry to your project or global `~/.cargo/config.toml`:

```toml
[registries.renop]
index = "sparse+http://localhost:3000/releases/index/"

# Optional: replace default crates.io upstream
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/releases/index/"
```

## 2. Authentication Token

To publish crates or access private crates, log in using your RenoP Personal Access Token:

```bash
cargo login --registry renop
# Paste your RenoP token when prompted
```

Alternatively, configure credentials directly in `~/.cargo/credentials.toml`:

```toml
[registries.renop]
token = "your_renop_token"
```

## 3. Dependency Management & Publishing

### Adding Dependencies (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### Publishing Crates

```bash
cargo publish --registry renop
```

The first publication reserves the normalized crate name. If the repository has upstream mirrors, RenoP rejects a
name already present on any applicable mirror. It also fails the publication safely when an upstream availability
check is inconclusive, so a temporary mirror outage cannot create a conflicting local package.

### Search & Yank

```bash
# Search crates
cargo search --registry renop my-crate

# Yank a version
cargo yank --registry renop --version 0.1.0 my-crate

# Unyank
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

## 4. Cargodoc Online Documentation

For crates uploaded with documentation, RenoP extracts and serves rustdoc HTML previews via the Cargodoc engine:

Access URL:
`http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
