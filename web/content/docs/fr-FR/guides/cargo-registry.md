---
title: Registre Cargo (Rust)
order: 2
category: Guides
description: Configuration de l'index Sparse Cargo, publication et Cargodoc
---

# Registre Cargo (Rust)

```toml
# .cargo/config.toml
[registries.renop]
index = "sparse+http://localhost:3000/releases/index/"
```

```bash
cargo login --registry renop
cargo publish --registry renop
```
