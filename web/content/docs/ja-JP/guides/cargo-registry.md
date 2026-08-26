---
title: Cargo (Rust) Registry
order: 2
category: ガイド
description: Cargo repository、Sparse Index、publication、ownership、Cargodoc
---

# Cargo (Rust) Registry ガイド

client 設定前に format `cargo` の repository を作成します。例の名前は `crates` です。RenoP は Cargo Sparse
Index を実装し、Git index clone なしで crate archive を stream します。

## Cargo 設定 (`.cargo/config.toml`)

```toml
[registries.renop]
index = "sparse+http://localhost:3000/crates/"

# Optional: replace default crates.io upstream
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/crates/"
```

本番は HTTPS を使用します。repository `config.json` が download/API route を通知します。private repository は
`auth-required` を設定し、index と crate read に credential が必要です。

## 認証

専用の expiring API Token を作成します。初回公開は通常 `repository:read`、`repository:publish`、
`package:create` を使います。archive/yank は `package:lifecycle`、owner 管理は `team:manage` を追加します。

```bash
cargo login --registry renop
# Paste your RenoP token when prompted
```

Cargo は `~/.cargo/credentials.toml` に保存します。

```toml
[registries.renop]
token = "your_renop_token"
```

Token は完全な `Authorization` 値です。RenoP は scope/target と現在の account、repository、package-team
permission を必ず交差します。

## 依存関係と公開

### 依存関係の追加 (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### crate の公開

```bash
cargo publish --registry renop
```

初回成功時に正規化名を予約し、publisher に L4 を与えます。ローカルまたは適用ミラーの同名は拒否します。
上流確認が確定しない場合は `503` で安全に失敗し、package を予約しません。後続 version は team の公開 level
が必要です。

### Search、yank、unyank

```bash
# Search crates
cargo search --registry renop my-crate

# Yank a version
cargo yank --registry renop --version 0.1.0 my-crate

# Unyank
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

owner は package page で L0-L4 collaborator と invitation を管理します。mirror crate は upstream 表示され、
local owner を持たず read-only です。

## Cargodoc

RenoP は rustdoc を検証して sandbox viewer に抽出します。`config.yaml` で Cargodoc と size limit を有効化します。

URL: `http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
