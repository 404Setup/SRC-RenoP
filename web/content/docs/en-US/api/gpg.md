---
title: GPG signatures
order: 5
category: API
description: Register signing keys and verify Maven artifact signatures
---

# GPG signatures

RenoP can verify detached OpenPGP signatures for Maven artifacts. GPG policy applies to `.jar`, `.pom`, and `.module`
files. A signature is stored only after it has been verified against a key registered for the uploading account.

## Configuration

Configure one to eight HTTPS key servers in `server.gpg.key_servers` in `config.yaml`. The same setting is available
through the `server.gpg` field of the settings API. RenoP uses these servers to resolve a key ID or fingerprint when a
user registers a key. See [Configuration overview](../configuration/overview.md) and [Settings](./settings.md).

Repositories opt into mandatory signatures with `require_gpg_signature: true`. The setting applies to the three
protected extensions above; checksum files and Maven metadata companions are handled as part of the same publication.
See [Repositories & mirrors](../configuration/repositories.md).

## Register a key

An authenticated user can register up to ten public keys in the profile:

| Method   | Endpoint                             | Result                               |
|----------|--------------------------------------|--------------------------------------|
| `GET`    | `/api/auth/profile/gpg`              | `GpgKeyList`                         |
| `POST`   | `/api/auth/profile/gpg`              | `GpgKeyDto`                          |
| `DELETE` | `/api/auth/profile/gpg/:fingerprint` | Empty `204` response                 |
| `GET`    | `/api/auth/profile/gpg/releases`     | `GpgReleaseList` publication history |

The `POST` body is `GpgKeyReferenceRequest` (`application/x-protobuf`):

```protobuf
syntax = "proto3";

message GpgKeyReferenceRequest {
  string key_id = 1;
}
```

Use a full fingerprint when a short key ID is ambiguous. The server stores the resolved public key in the database and
does not accept private key material. These endpoints require an authenticated account; the release history belongs only
to the requesting user.

## Upload a signed artifact

Upload the artifact and its detached signature under the same Maven path. The signature filename must be the artifact
filename followed by the lowercase `.asc` suffix, for example `demo-1.0.0.jar.asc`.

For a single-request artifact upload, set `X-RenoP-GPG-Signature-Expected: true` when the matching signature is also
being uploaded:

```bash
curl -u alice:TOKEN \
  -H 'X-RenoP-GPG-Signature-Expected: true' \
  -T demo-1.0.0.jar \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar'

curl -u alice:TOKEN \
  -T demo-1.0.0.jar.asc \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar.asc'
```

For a chunked upload, set `gpg_signature_expected: true` in `ChunkedUploadInitRequest` instead of using the HTTP header.
The browser upload form sets this flag automatically when it detects a matching `.asc` file.

The detached signature must be an ASCII-armored OpenPGP signature no larger than 1 MiB. The signing key must be one of
the keys registered by the uploader. The artifact remains in the GPG quarantine until the pair has been validated when
the repository requires a signature or the upload explicitly expects one. A missing pair expires after approximately 15
minutes and is recorded as a failed publication.

If signatures are optional and the artifact is uploaded without the expectation flag, the artifact is published as an
unsigned file. A later `.asc` upload can still create a verified signature record. Uploading a replacement invalidates
the previous record unless the new publication is verified.

## Inspect verification results

### `GET /api/maven/signatures/:repo_name/*`

Returns `GpgSignatureDetails` (`application/x-protobuf`) for a verified `.jar`, `.pom`, or `.module` artifact. The
request requires read permission for the repository. A missing record, an unsupported extension, or an inaccessible
artifact returns `404`.

Important fields:

| Field                                     | Meaning                                       |
|-------------------------------------------|-----------------------------------------------|
| `repository` / `artifact_path`            | Repository and Maven-relative artifact path   |
| `fingerprint` / `key_id`                  | Signing public key identifiers                |
| `primary_identity`                        | Primary identity from the resolved public key |
| `uploader`                                | Account that submitted the publication        |
| `signature_created_at` / `verified_at`    | Unix timestamps in milliseconds               |
| `hash_algorithm` / `public_key_algorithm` | Algorithms recorded during verification       |

`FileDetails.signed` is `true` only when a verified signature record exists for that file. The browser shows a lock
action for signed artifacts; selecting it loads the endpoint above. Text preview remains available for supported text
files, including `.pom` and `.module` artifacts.

Publication failures and their reason are available to the uploader through `GET /api/auth/profile/gpg/releases`.
Possible statuses are `queued`, `validating`, `success`, and `failed`.
