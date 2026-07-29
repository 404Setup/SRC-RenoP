---
title: Stockage
order: 8
category: API
---

# Chemins de stockage des dépôts

Les chemins d’artefacts ne sont pas sous `/api`. Disposition :

```text
/{repo_name}/{maven-path}
```

Dépôts par défaut :

```text
/releases/...
/snapshots/...
/private/...
```

Les noms de dépôt ne doivent pas entrer en collision avec les routes statiques telles que `api`, `js`, `css`, `svg`, `assets` ou `javadocs`.

## Méthodes

| Méthode    | Permission | Comportement                                                                  |
|------------|------------|-------------------------------------------------------------------------------|
| GET        | lecture    | Téléchargement ; une Accept HTML navigateur peut retomber sur le SPA de gestion |
| HEAD       | lecture    | En-têtes de réponse uniquement                                                |
| PUT / POST | écriture   | Upload ou écrasement                                                          |
| DELETE     | écriture   | Suppression ; statut de succès `204`                                          |

La taille maximale du corps est d’environ 2 GiB (`BodyLimit`). Les uploads sont streamés.

### Upload

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Un upload réussi renvoie `201 Created`. Si le redeploy est désactivé et que l’objet existe déjà, la requête échoue avec un statut non 2xx.

L’en-tête optionnel `X-Generate-Checksums: true` écrit les sidecars `.md5`, `.sha1`, `.sha256` et `.sha512`.

Le serveur met à jour l’index d’artefacts, les checksums optionnels et la synchronisation S3 selon la configuration. Les clients voient une disposition de dépôt Maven standard.

### Upload découpé (optionnel)

L’authentification est la même que pour l’écriture storage : cookie de session, Basic ou Bearer, avec permission d’écriture sur le dépôt cible.

Préfixe : `/api/upload/chunked`

L’UI navigateur utilise l’upload découpé pour les fichiers de **8 MiB** ou plus ; les fichiers plus petits utilisent un seul `PUT`. Les clients non navigateur peuvent ouvrir une session découpée pour toute taille. Le serveur peut regrouper les très petites charges en une seule partie.

Init et complete utilisent **`application/x-protobuf`** (`ChunkedUploadInitRequest`, `ChunkedUploadInitResponse` et `ChunkedUploadCompleteResponse` dans `proto/api/v1/api.proto`). Les corps de parties sont en binaire brut.

1. **`POST /api/upload/chunked/`** — créer une session (`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`)

| Champ                | Description                                        |
|----------------------|----------------------------------------------------|
| `purpose`            | `storage` (défaut)                                 |
| `path`               | Chemin de destination `repo/…/file`                |
| `filename`           | Nom d’affichage optionnel                          |
| `size`               | Taille totale en octets                            |
| `generate_checksums` | Écrire les sidecars de checksum                    |
| `chunk_size`         | Taille de partie préférée (optionnel ; normalisé)  |

Champs de réponse : `upload_id`, `chunk_size`, `chunk_count`, `purpose`. Les uploads de parties suivants doivent utiliser les `chunk_size` et `chunk_count` renvoyés.

**Règles de taille de partie** (serveur, `upload.NormalizeChunkSize`) :

| Taille totale | Taille de partie                   |
|---------------|------------------------------------|
| ≤ 256 KiB     | Une partie égale à la taille fichier |
| ≤ 8 MiB       | Une partie égale à la taille fichier |
| ≤ 32 MiB      | 4 MiB                              |
| ≤ 128 MiB     | 8 MiB                              |
| ≤ 512 MiB     | 16 MiB                             |
| ≤ 2 GiB       | 24 MiB                             |
| plus grand    | 32 MiB (maximum)                   |

Le `chunk_size` fourni par le client est borné à **256 KiB … 32 MiB**. Si le nombre de parties dépasserait environ 2048, le serveur augmente la taille de partie. Omettre `chunk_size` ou envoyer `0` pour utiliser le tableau ci-dessus.

2. **`PUT /api/upload/chunked/:upload_id/:index`** — corps de partie brut (index 0-based) ; uploads parallèles autorisés  
   Succès : `204`. Le re-upload d’un index déjà accepté est idempotent.

3. **`POST /api/upload/chunked/:upload_id/complete`** — assemblage, mise à jour d’index, checksums optionnels  
   Succès : `201` avec `ChunkedUploadCompleteResponse` (`status=created`, `path=…`).

4. **`DELETE /api/upload/chunked/:upload_id`** — abandonner la session et jeter les données temporaires (`204`).

Les sessions incomplètes expirent après environ **15 minutes** ; les données temporaires sont supprimées.

### Téléchargement

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

Les dépôts PUBLIC ne nécessitent pas d’authentification. Les dépôts PRIVATE exigent Basic ou Bearer.

Avec des miroirs configurés, les objets absents localement peuvent être récupérés en amont selon le cache et le negative-cache du dépôt.

### Suppression

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## Accès navigateur

Avec `Accept: text/html`, les dépôts manquants ou certains répertoires retombent sur le SPA de gestion, de sorte que des chemins tels que `http://host/releases/...` peuvent ouvrir l’UI. Les clients machine doivent envoyer `Accept: */*` ou omettre `Accept` pour éviter les réponses HTML.

## Aperçu Javadoc

Lorsque c’est activé :

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

Nécessite la permission de lecture correspondante. La forme `raw` sert les fichiers contenus dans le jar. La taille est limitée par `max_javadoc_size_mb`.

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

Dans `~/.m2/settings.xml`, définir username et password (ou upload token) pour le `id` server correspondant.
