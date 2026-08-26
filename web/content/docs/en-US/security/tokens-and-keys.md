---
title: Tokens & GPG Signatures
order: 2
category: Security
description: Fine-grained API tokens and OpenPGP signature verification
---

# Tokens & GPG Signatures

RenoP provides token-based authentication for automation pipelines and OpenPGP signature verification for artifact
integrity.

## 1. Fine-grained API tokens

Create and revoke API tokens from the account profile. Each token has a private name, one or more capability scopes,
and an optional expiration. The 256-bit secret is displayed once; only its SHA-256 digest is persisted.

Use the minimum scopes and shortest practical lifetime for each workstation or automation pipeline. A token authorizes
an operation only while both conditions remain true:

1. The token carries the required capability scope.
2. Its owning account still has the necessary system, repository, domain, or package-team permission.

Administrator scopes are offered only to administrator accounts and do not preserve access after that role is removed.
Revocation invalidates cached authentication immediately. Legacy plaintext upload tokens are migrated automatically to
hashed `repository:read` and `repository:publish` credentials.

Browser session secrets are accepted only through the HttpOnly `renop_session` cookie. Basic credentials are limited
to standard package protocols. API automation should send a fine-grained token as `Authorization: Bearer <token>`;
credentials in URL query parameters are not accepted.

---

## 2. GPG Detached Signatures & Verification

To verify that Maven artifacts have not been tampered with, RenoP supports detached OpenPGP (`.asc`) signature
verification.

### Enforcing GPG Signatures

Set `require_gpg_signature: true` in `repositories.yaml`:

```yaml
repositories:
  releases:
    name: releases
    require_gpg_signature: true
```

### Verification Flow

1. An artifact (e.g. `mylib-1.0.0.jar`) is uploaded.
2. If the `.asc` signature is missing, RenoP holds the artifact in the quarantine queue (`.renop.tmp.gpg`).
3. When the matching `mylib-1.0.0.jar.asc` is uploaded, RenoP retrieves the corresponding public key from registered
   keyservers or the user's profile and validates the cryptographic signature.
4. Upon successful verification, the artifact and signature are released to the public repository.
