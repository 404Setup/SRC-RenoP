---
title: Maven
order: 4
category: API
---

# Navigation Maven et aides

Préfixe : `/api/maven` (badge sous `/api/badge`)

Ces points d’accès lisent l’index et les métadonnées. Les octets d’artefacts sont sous `/{repo}/group/artifact/…` —
voir [storage.md](./storage.md).

Les paramètres de chemin suivent la disposition Maven, par ex. :

```text
com/example/demo
com/example/demo/1.0.0
```

Un droit de lecture insuffisant donne en général `404 Not found`.

## Détails répertoires et fichiers (Protobuf)

### `GET /api/maven/details`

Dépôts visibles pour l’utilisateur courant, enveloppés comme racine virtuelle.

Réponse : `FileDetails` (`application/x-protobuf`)

```text
type = DIRECTORY
name = "repositories"
files[] = { type: DIRECTORY, name: "<repo>" }
```

### `GET /api/maven/details/:repo_name`

Racine du dépôt (avec enfants).

### `GET /api/maven/details/:repo_name/*`

Détails du chemin. Les répertoires incluent `files` ; les fichiers incluent `content_length` et `last_modified_time`
(RFC3339Nano).

`type` est `FILE` ou `DIRECTORY`.

### `GET /api/maven/repo-details/:repo_name`

Statistiques et résumé des miroirs. Réponse : `RepoDetailsResponse`.

| Champ                                               | Signification                                                   |
|-----------------------------------------------------|-----------------------------------------------------------------|
| `name` / `visibility`                               | Nom, visibilité                                                 |
| `total_size` / `artifact_size` / `metadata_size`    | Octets                                                          |
| `total_files` / `artifact_count` / `metadata_count` | Compteurs (checksums et maven-metadata comptent comme metadata) |
| `mirrors[]`                                         | name, url, persist, cache_ttl, negative_cache, …                |

Pas d’accès en lecture → **403** (contrairement à details, souvent en 404).

## Requêtes de versions (Protobuf)

Le chemin doit pointer vers un répertoire de coordonnées avec `maven-metadata.xml` (groupId/artifactId).

### `GET /api/maven/versions/:repo_name/*`

| Query    | Défaut | Signification                 |
|----------|--------|-------------------------------|
| `filter` | —      | Filtre sous-chaîne de version |
| `sorted` | `true` | Trier les résultats           |

Réponse : `application/x-protobuf`, `VersionsResponse`

```protobuf
message VersionsResponse {
  bool is_snapshot = 1;
  repeated string versions = 2;
}
```

### `GET /api/maven/latest/version/:repo_name/*`

Mêmes query ; ajoutez `type=raw` pour une chaîne de version brute (`text/plain`).

Réponse par défaut : `application/x-protobuf`, `LatestVersionResponse`

```protobuf
message LatestVersionResponse {
  bool is_snapshot = 1;
  string version = 2;
}
```

### `GET /api/maven/latest/details/:repo_name/*`

`FileDetails` pour le dernier artefact correspondant (`application/x-protobuf`).

| Query        | Défaut | Signification     |
|--------------|--------|-------------------|
| `extension`  | `jar`  | Extension         |
| `classifier` | —      | Classifier        |
| `filter`     | —      | Filtre de version |

### `GET /api/maven/latest/file/:repo_name/*`

Résout la dernière version, puis récupère via la couche stockage (redirection ou corps — proche d’une URL d’artefact
directe).

## Badge

### `GET /api/badge/latest/:repo_name/*`

Badge SVG avec la dernière version. `Content-Type: image/svg+xml`.

| Query    | Signification                             |
|----------|-------------------------------------------|
| `name`   | Libellé gauche (défaut : nom du dépôt)    |
| `color`  | Couleur droite (alphanumérique ou `#hex`) |
| `prefix` | Préfixe de version                        |
| `filter` | Filtre de version                         |

```markdown
![latest](https://your-host/api/badge/latest/releases/com/example/demo)
```

## Générer un POM

### `POST /api/maven/generate/pom/:repo_name/*`

Nécessite un accès en écriture au dépôt. Corps : `application/x-protobuf`, `PomDetails` (accepte aussi JSON).

```protobuf
message PomDetails {
  string group_id = 1;
  string artifact_id = 2;
  string version = 3;
}
```

Le chemin peut déjà se terminer par `.pom`, ou être un répertoire de coordonnées (alors le nom de fichier est
`artifact_id-version.pom`).

Disque insuffisant → 507. En cas de succès le POM est écrit et l’index mis à jour.

## Politique de confidentialité

### `GET|HEAD /api/privacy-policy`

Si `privacy-policy.txt` existe dans le répertoire de travail, le renvoyer en `text/plain` ; sinon 404. Sans lien avec
Maven ; monté sur le même groupe d’API.
