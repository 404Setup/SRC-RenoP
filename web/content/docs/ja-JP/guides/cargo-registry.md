---
title: Cargo (Rust) レジストリ
order: 2
category: クライアントガイド
description: Cargo スパースインデックスの設定、Crate の公開および Cargodoc
---

# Cargo (Rust) レジストリ設定

## 1. Cargo の設定 (`.cargo/config.toml`)

```toml
[registries.renop]
index = "sparse+http://localhost:3000/releases/index/"
```

## 2. 認証トークン

```bash
cargo login --registry renop
# トークンを入力
```

## 3. Crate の公開と操作

```bash
# 公開
cargo publish --registry renop

# 検索
cargo search --registry renop my-crate

# ヤンク (yank)
cargo yank --registry renop --version 0.1.0 my-crate
```

## 4. Cargodoc ドキュメントプレビュー

`http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
