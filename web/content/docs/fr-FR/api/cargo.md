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

Pour une publication locale, RenoP lit la déclaration `package.readme` du `Cargo.toml` validé et extrait ce fichier de
l’archive sans mettre le crate en mémoire. La réponse détaillée expose au plus 512 Kio de Markdown, rendu par la liste
commune d’éléments et d’URL autorisés. Les catalogues et recherches ne chargent pas les README.

---

## Télécharger une crate

- **Chemin** : `GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **Réponse** : archive `.crate` avec `application/x-tar`.

---

## Yank et unyank

- **Yank** : `DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank** : `PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **Authentification** : propriétaire de la crate ou administrateur.

## Verrouillage des ressources

Les administrateurs et modérateurs de ce dépôt utilisent `PUT /{repo}/api/v1/crates/{crate_name}/locks` avec
le cookie d'une session de navigateur active. Les jetons API et identifiants des clients de paquets sont exclus.

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

Omettez `version` ou utilisez `""` pour verrouiller le paquet. Les modes sont `write` et `read`. Les motifs
publics traduits sont `hold`, `prohibited`, `expired`, `trojan`, `abuse`, `dmca`, `reup`, `squatting` et `quality`.
Une modification réussie renvoie `{"ok":true}`. Pour supprimer le verrouillage manuel exact, envoyez
`DELETE /{repo}/api/v1/crates/{crate_name}/locks` avec `{"version":"1.2.3"}`.

Le mode écriture bloque les modifications et actualisations amont, mais conserve les téléchargements déjà stockés.
Le mode lecture interdit aussi tout téléchargement de fichier et aperçu documentaire, même au personnel.
Les métadonnées restent accessibles aux administrateurs, modérateurs du dépôt, propriétaires et collaborateurs,
y compris L0 et membres des équipes globales liées. Les autres visiteurs ne voient pas la ressource verrouillée
dans les métadonnées, index sparse, recherches, profils ou listes de ressources des équipes.

Les métadonnées de paquet et de version exposent `locks`, avec `mode`, `reason`, `source` et `locked_at`.
Les verrouillages système et manuels sont indépendants. Supprimer le manuel conserve les restrictions système.
Les mutations refusées renvoient `423` et `X-Renop-Error-Code: resource_locked`, les lectures refusées `404`.
Une version verrouillée empêche l'archivage, l'abandon définitif et la suppression du paquet entier, mais permet
la publication d'autres versions. La reconfiguration et la suppression du dépôt sont bloquées tant qu'il contient des verrous.
