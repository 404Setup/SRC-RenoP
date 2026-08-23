---
title: Tokens & GPG Signatures
order: 2
category: Security
description: Personal Access Tokens (PAT), upload tokens, and OpenPGP signature verification
---

# Tokens & GPG Signatures

RenoP provides token-based authentication for automation pipelines and OpenPGP signature verification for artifact
integrity.

## 1. Access Token Types

Create and manage tokens in the Web console under "Token Management":

### Personal Access Tokens (PAT)

- Bound to an individual user, inheriting the user's active permissions.
- Ideal for developer workstations (`settings.xml`, `.cargo/credentials.toml`, Docker CLI).
- Supports expiration dates and immediate revocation.

### Upload Tokens

- Designed for CI/CD runners (GitHub Actions, GitLab CI, Jenkins).
- Scoped strictly to specific repositories for artifact deployment, preventing unauthorized reads or administrative API
  calls.

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
