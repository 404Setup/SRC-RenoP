---
title: Signatures GPG
order: 5
category: API
description: Enregistrer des clés de signature et vérifier les signatures des artefacts Maven
---

# Signatures GPG

RenoP vérifie les signatures OpenPGP détachées des artefacts Maven. La politique GPG s’applique aux fichiers `.jar`,
`.pom` et `.module`. Une signature n’est enregistrée qu’après sa vérification avec une clé enregistrée par le compte qui
effectue le téléversement.

## Configuration

Configurez de un à huit serveurs de clés HTTPS dans `server.gpg.key_servers` de `config.yaml`. Le même réglage est
accessible via le champ `server.gpg` de l’API des paramètres. Ces serveurs servent à résoudre un identifiant de clé ou
une empreinte lors de l’enregistrement d’une clé. Consultez
la [vue d’ensemble de la configuration](../configuration/overview.md)
et l’[API des paramètres](./settings.md).

Un dépôt peut imposer les signatures avec `require_gpg_signature: true`. Cette option concerne les trois extensions
protégées ; les fichiers de somme de contrôle et les métadonnées Maven sont traités dans la même publication. Voir
[Dépôts et miroirs](../configuration/repositories.md).

## Enregistrer une clé

Un utilisateur authentifié peut enregistrer jusqu’à dix clés publiques dans son profil :

| Méthode  | Point d’accès                        | Résultat                    |
|----------|--------------------------------------|-----------------------------|
| `GET`    | `/api/auth/profile/gpg`              | `GpgKeyList`                |
| `POST`   | `/api/auth/profile/gpg`              | `GpgKeyDto`                 |
| `DELETE` | `/api/auth/profile/gpg/:fingerprint` | Réponse `204` vide          |
| `GET`    | `/api/auth/profile/gpg/releases`     | Historique `GpgReleaseList` |

Le corps du `POST` est un `GpgKeyReferenceRequest` (`application/x-protobuf`) :

```protobuf
syntax = "proto3";

message GpgKeyReferenceRequest {
  string key_id = 1;
}
```

Si un identifiant court correspond à plusieurs clés, utilisez l’empreinte complète. Le serveur conserve la clé publique
résolue dans la base de données et n’accepte jamais de clé privée. Ces points d’accès nécessitent un compte
authentifié ; l’historique des publications n’est visible que par l’utilisateur concerné.

## Téléverser un artefact signé

Téléversez l’artefact et sa signature détachée sous le même chemin Maven. Le nom de la signature doit reprendre le nom
de l’artefact et ajouter le suffixe `.asc` en minuscules, par exemple `demo-1.0.0.jar.asc`.

Pour un téléversement en une seule requête, envoyez `X-RenoP-GPG-Signature-Expected: true` lorsque la signature
correspondante fait partie du même lot :

```bash
curl -u alice:TOKEN \
  -H 'X-RenoP-GPG-Signature-Expected: true' \
  -T demo-1.0.0.jar \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar'

curl -u alice:TOKEN \
  -T demo-1.0.0.jar.asc \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar.asc'
```

Pour un téléversement découpé, renseignez `gpg_signature_expected: true` dans `ChunkedUploadInitRequest` au lieu de
utiliser l’en-tête HTTP. Le formulaire de téléversement du navigateur renseigne automatiquement ce champ lorsqu’il
détecte un fichier `.asc` correspondant.

La signature doit être une signature OpenPGP ASCII-armored de 1 MiB au maximum. La clé de signature doit être
enregistrée par l’utilisateur qui téléverse le fichier. Lorsque le dépôt impose une signature, ou lorsque le
téléversement l’annonce explicitement, l’artefact reste dans la zone de quarantaine GPG jusqu’à la validation de la
paire. Une paire incomplète expire après environ 15 minutes et est enregistrée comme publication en échec.

Si les signatures sont facultatives et que l’artefact n’est pas téléversé avec l’indicateur d’attente, il est publié
comme fichier non signé. Un fichier `.asc` envoyé ultérieurement peut encore créer un enregistrement de signature
vérifié. Le remplacement d’un artefact invalide l’enregistrement précédent, sauf si la nouvelle publication est validée.

## Consulter le résultat de la vérification

### `GET /api/maven/signatures/:repo_name/*`

Renvoie `GpgSignatureDetails` (`application/x-protobuf`) pour un artefact `.jar`, `.pom` ou `.module` dont la signature
est vérifiée. La lecture du dépôt est requise. L’absence d’enregistrement, une extension non prise en charge ou un
artefact inaccessible produit une réponse `404`.

| Champ                                     | Signification                                 |
|-------------------------------------------|-----------------------------------------------|
| `repository` / `artifact_path`            | Dépôt et chemin Maven relatif de l’artefact   |
| `fingerprint` / `key_id`                  | Identifiants de la clé publique de signature  |
| `primary_identity`                        | Identité principale de la clé résolue         |
| `uploader`                                | Compte ayant soumis la publication            |
| `signature_created_at` / `verified_at`    | Horodatages Unix en millisecondes             |
| `hash_algorithm` / `public_key_algorithm` | Algorithmes consignés lors de la vérification |

`FileDetails.signed` vaut `true` uniquement lorsqu’un enregistrement de signature vérifié existe pour le fichier. Le
navigateur affiche alors une action de verrouillage ; son activation ouvre la boîte de dialogue des détails de la
signature et charge le point d’accès ci-dessus. L’aperçu texte reste disponible pour les fichiers pris en charge,
notamment les artefacts `.pom` et `.module`.

Les échecs de publication et leur motif sont accessibles à l’utilisateur via `GET /api/auth/profile/gpg/releases`. Les
états possibles sont `queued`, `validating`, `success` et `failed`.
