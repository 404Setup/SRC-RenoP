---
title: Stockage
order: 8
category: API
---
# Chemins de stockage des dépôts

Les artefacts ne sont pas sous `/api`. Disposition :

```text
/{repo_name}/{maven-path}
```

Dépôts par défaut :

```text
/releases/...
/snapshots/...
/private/...
```

Les noms de dépôts ne doivent pas entrer en collision avec les routes statiques : `api`, `js`, `css`, `svg`, `assets`, `javadocs`, etc.

## Méthodes

| Méthode     | Permission | Comportement                                                            |
|------------|------------|---------------------------------------------------------------------|
| GET        | lecture       | Téléchargement ; les requêtes HTML navigateur peuvent basculer vers le SPA d’admin |
| HEAD       | lecture       | En-têtes seulement                                                        |
| PUT / POST | écriture      | Upload / écrasement                                                  |
| DELETE     | écriture      | Suppression ; succès 204                                                 |

Limite de corps d’environ 2 GiB (`BodyLimit`) ; les uploads sont streamés.

### Upload

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Succès typique : `201 Created`. Si le redeploy est désactivé et que le fichier existe, le serveur refuse l’écrasement (traitez tout
non-2xx comme un échec).

En-tête optionnel : `X-Generate-Checksums: true` écrit les sidecars `.md5` / `.sha1` / `.sha256` / `.sha512`.

Le serveur maintient l’index, les checksums optionnels et la sync S3. Les clients Maven voient une disposition de dépôt normale.

### Upload multi-parties (chunked) — optionnel

Le `PUT` monorequête ci-dessus est inchangé. Pour les gros fichiers, l’UI web peut utiliser un upload découpé concurrent
(avec retries par partie). Les clients machine peuvent utiliser la même API.

**Quand utiliser multi-part :** l’UI navigateur ne découpe pas sous **8 MiB** (un seul `PUT` est plus rapide). Les clients
machine peuvent ouvrir une session découpée pour toute taille ; le serveur regroupe les très petits payloads en une partie.

Préfixe : `/api/upload/chunked` (cookie de session / Basic / Bearer ; besoin d’écriture sur le dépôt cible).

Init et complete utilisent **`application/x-protobuf`** (`ChunkedUploadInitRequest` /
`ChunkedUploadInitResponse` / `ChunkedUploadCompleteResponse` dans `proto/api/v1/api.proto`). Corps des parties en binaire brut.

1. **`POST /api/upload/chunked/`** — démarrer une session (`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`)

Champs logiques (snake_case) :

| Champ                | Signification                                           |
|----------------------|---------------------------------------------------|
| `purpose`            | `storage` (défaut)                               |
| `path`               | Destination `repo/…/file`                         |
| `filename`           | Nom d’affichage optionnel                             |
| `size`               | Octets totaux                                       |
| `generate_checksums` | Écrire les sidecars de checksum                           |
| `chunk_size`         | Taille de partie préférée (optionnel ; normalisé côté serveur) |

Champs de réponse : `upload_id`, `chunk_size`, `chunk_count`, `purpose`. Utilisez toujours les
`chunk_size` / `chunk_count` renvoyés pour les `PUT` suivants.

**Règles de taille de partie** (serveur, `upload.NormalizeChunkSize`) :

| Taille totale | Taille de partie typique       |
|------------|-------------------------|
| ≤ 256 KiB  | Une partie = taille du fichier |
| ≤ 8 MiB    | Une partie = taille du fichier |
| ≤ 32 MiB   | 4 MiB                   |
| ≤ 128 MiB  | 8 MiB                   |
| ≤ 512 MiB  | 16 MiB                  |
| ≤ 2 GiB    | 24 MiB                  |
| plus grand     | 32 MiB (max)            |

Le `chunk_size` client est borné à **256 KiB … 32 MiB**. S’il créerait plus de ~2048 parties, le serveur augmente
la taille de partie. Omettez `chunk_size` (ou envoyez `0`) pour accepter le tableau ci-dessus.

2. **`PUT /api/upload/chunked/:upload_id/:index`** — corps de partie brut (base 0), parallèle OK  
   Succès : `204`. Re-PUT d’un index déjà accepté est idempotent (sûr pour retry).

3. **`POST /api/upload/chunked/:upload_id/complete`** — assembler, indexer, checksums optionnels  
   Succès : `201` + `ChunkedUploadCompleteResponse` (`status=created`, `path=…`).

4. **`DELETE /api/upload/chunked/:upload_id`** — abandonner et jeter les temps (`204`).

Les sessions expirent après environ **15 minutes** sans complete (temps supprimés). Les clients doivent retenter les parties en échec avec backoff.

### Téléchargement

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC sans auth. PRIVATE via Basic / Bearer.

Avec des miroirs, les fichiers absents localement peuvent être récupérés en amont (cache / negative-cache selon la
config du dépôt).

### Suppression

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## Accès navigateur

Avec `Accept: text/html`, les dépôts manquants ou certains répertoires basculent vers le SPA d’admin pour que
`http://host/releases/...` ouvre l’UI. Les clients machine devraient utiliser `Accept: */*` ou omettre Accept pour éviter le HTML.

## Aperçu Javadoc

Quand activé :

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

Nécessite la permission de lecture correspondante. `raw` sert les fichiers dans le jar. Taille limitée par `max_javadoc_size_mb`.

## Exemple de configuration Maven

```xml

<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>

<distributionManagement>
<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>
<snapshotRepository>
    <id>renop</id>
    <url>http://localhost:3000/snapshots</url>
</snapshotRepository>
</distributionManagement>
```

Dans `~/.m2/settings.xml`, définissez username + password (ou jeton d’upload) pour l’id serveur.
