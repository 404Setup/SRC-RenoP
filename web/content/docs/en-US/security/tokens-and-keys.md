---
title: Tokens & GPG Signatures
order: 2
category: Security
description: Fine-grained machine credentials, recovery material, and OpenPGP publication verification
---

# Tokens & GPG Signatures

RenoP separates browser sessions, API Token, password authentication, recovery material, and artifact signing keys.
They have different storage, transport, and revocation rules.

## API Token and recovery material

API Token use 256 random bits and an `rnp_pat_` prefix. The secret is shown once; only its SHA-256 lookup digest is
stored. Each Token has a private label, one or more capability scopes, optional exact repository/package/team/domain
targets, and an optional expiration. Accounts may own at most 50 Token and each Token at most 128 targets.

Use the least privilege and shortest practical lifetime. Authorization requires both the Token policy and its account's
current system, repository, domain, or package-team permission. Revocation clears authentication caches immediately.
Legacy plaintext upload tokens migrate to hashed compatibility credentials.

Browser session secrets are cookie-only. Basic credentials are package-protocol-only. API automation sends
`Authorization: Bearer <token>`. Credentials in query strings are ignored or rejected.

Recovery codes are separate from API Token. A generated set contains twelve one-time high-entropy codes; RenoP stores
Argon2id verifiers. Four distinct unused codes reset the password atomically, consume those codes, revoke sessions, and
re-enable password login. Store codes offline and replace the set after use or suspected disclosure.

---

## Detached OpenPGP verification

Maven repositories can require a valid detached `.asc` signature before an artifact becomes visible. Users register
public keys in their account; private keys never enter RenoP.

### Enable verification

```yaml
repositories:
  releases:
    name: releases
    format: maven
    require_gpg_signature: true
```

### Publication flow

1. RenoP streams the artifact into `.renop.tmp.gpg` and creates a bounded pending release.
2. The matching `.asc` may arrive before or after the artifact within the publication deadline.
3. RenoP resolves an unambiguous registered fingerprint, verifies the signature and uploader authorization, and rechecks
   repository/domain policy under the repository gate.
4. A successful artifact/signature pair is committed atomically and its verified metadata is stored for the UI.
5. Invalid, missing, expired, deleted, or unauthorized releases fail with a stable reason in audit/profile history.

Key-server URLs must use HTTPS and are configured globally under `server.gpg.key_servers`. Outbound requests follow the
selected proxy policy, use bounded clients, and never upload a private key.
