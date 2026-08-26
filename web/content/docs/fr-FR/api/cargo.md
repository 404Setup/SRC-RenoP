---
title: API du registre Cargo
order: 5
category: Référence API
description: Index clairsemé, publication, téléchargement et retrait de crates
---

# API du registre Cargo

RenoP implémente les spécifications Cargo Registry et Sparse Index.

## Configuration de l’index (`config.json`)

- **Chemin** : `GET /{repo}/config.json` ou `GET /{repo}/index/config.json`
- **Usage** : Cargo lit ce document lors de la première connexion afin de découvrir les routes du registre.

### Réponse JSON

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## Métadonnées Sparse Index

- **Chemin** : `GET /{repo}/index/{prefix}/{crate_name}`
- **Usage** : renvoie du JSON délimité par lignes selon le partitionnement officiel des noms de crates.

---

## Publier une crate

- **Chemin** : `PUT /{repo}/api/v1/crates/new`
- **Authentification** : Token dans `Authorization: <token>`.
- **Corps** : longueur JSON sur 4 octets, métadonnées JSON, puis archive binaire `.crate`.
- **Conflit de nom** : la première publication renvoie `409 Conflict` si le nom normalisé existe localement ou sur un
  miroir applicable. Une vérification amont indéterminée renvoie `503 Service Unavailable`.

---

## Télécharger une crate

- **Chemin** : `GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **Réponse** : archive `.crate` avec `application/x-tar`.

---

## Yank et unyank

- **Yank** : `DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank** : `PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **Authentification** : propriétaire de la crate ou administrateur.
