---
title: Реестр Cargo (Rust)
order: 2
category: Руководства
description: Настройка Sparse Index, публикация пакетов и просмотр Cargodoc
---

# Реестр Cargo (Rust)

```toml
# .cargo/config.toml
[registries.renop]
index = "sparse+http://localhost:3000/releases/index/"
```

```bash
# Вход и публикация
cargo login --registry renop
cargo publish --registry renop
```
