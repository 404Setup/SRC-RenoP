---
title: Registre Cargo (Rust)
order: 2
category: Guides
description: Créer un dépôt Cargo, configurer Sparse Index, publier, gérer la propriété et Cargodoc
---

# Guide du registre Cargo (Rust)

Créez un dépôt de format `cargo` avant de configurer le client. Les exemples utilisent `crates`. RenoP implémente
Sparse Index et diffuse les archives sans cloner un index Git.

## Configurer Cargo (`.cargo/config.toml`)

```toml
[registries.renop]
index = "sparse+http://localhost:3000/crates/"

# Optional: replace default crates.io upstream
# [source.crates-io]
# replace-with = "renop"
# [source.renop]
# registry = "sparse+http://localhost:3000/crates/"
```

Utilisez HTTPS en production. `config.json` annonce les routes de téléchargement et d’API. Un dépôt privé active
`auth-required` et exige les identifiants pour l’index et les crates.

## Authentification

Créez un API Token dédié et expirant. La première publication utilise en général `repository:read`,
`repository:publish` et `package:create`. Ajoutez `package:lifecycle` pour archive/yank ou `team:manage` pour les
membres.

```bash
cargo login --registry renop
# Paste your RenoP token when prompted
```

Cargo l’enregistre dans `~/.cargo/credentials.toml` :

```toml
[registries.renop]
token = "your_renop_token"
```

Le Token est la valeur complète de `Authorization`. RenoP croise toujours scopes et cibles avec les droits actuels du
compte, du dépôt et de l’équipe.

## Dépendances et publication

### Ajouter une dépendance (`Cargo.toml`)

```toml
[dependencies]
my-crate = { version = "0.1.0", registry = "renop" }
```

### Publier une crate

```bash
cargo publish --registry renop
```

La première publication réserve le nom normalisé et donne L4 à l’éditeur. Un nom local ou présent sur un miroir
applicable est refusé. Une vérification amont indéterminée renvoie `503` sans réserver le paquet. Les versions suivantes
exigent le niveau de publication de l’équipe.

Lorsque l’examen des publications est activé, `cargo publish` renvoie `202 Accepted` après le stockage de l’archive.
Le crate reste absent de l’index sparse et du catalogue public jusqu’à son approbation par un modérateur du dépôt ou un
administrateur système. Avec `new_packages`, cette règle s’applique jusqu’à la première version visible. Les crates
issus
d’un miroir ne sont pas soumis à cet examen.

### Rechercher, yank et unyank

```bash
# Search crates
cargo search --registry renop my-crate

# Yank a version
cargo yank --registry renop --version 0.1.0 my-crate

# Unyank
cargo yank --registry renop --undo --version 0.1.0 my-crate
```

Les propriétaires gèrent collaborateurs L0-L4 et invitations depuis la page du paquet. Les crates miroir sont marquées
comme amont, sans propriétaire local, et restent en lecture seule.

## Cargodoc

RenoP valide et extrait rustdoc dans un viewer sandboxé. Activez Cargodoc et ses limites dans `config.yaml`.

URL : `http://localhost:3000/cargodoc/{repo}/{crate}/{version}/index.html`
