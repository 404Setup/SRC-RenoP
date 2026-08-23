---
title: GPG Cryptography API
order: 11
category: API Reference
description: User OpenPGP public key management and signature validation endpoints
---

# GPG Cryptography API

## 1. List User GPG Keys

- **Path**: `GET /api/auth/profile/gpg`
- **Auth**: Required

### Response (JSON)

```json
{
  "keys": [
    {
      "key_id": "9B27346A83C1D0EE",
      "fingerprint": "A518767AE71A1C38BCE3178C9B27346A83C1D0EE",
      "user_id": "Developer <dev@example.com>",
      "created_at": 1740000000
    }
  ]
}
```

---

## 2. Register GPG Public Key

- **Path**: `POST /api/auth/profile/gpg`
- **Auth**: Required
- **Request Body (JSON)**:
  ```json
  {
    "public_key_armored": "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n-----END PGP PUBLIC KEY BLOCK-----"
  }
  ```
- **Response**: `200 OK` with parsed key metadata.

---

## 3. List Quarantined Artifacts

- **Path**: `GET /api/auth/profile/gpg/releases`
- **Description**: Lists uploaded artifacts currently held in the `.renop.tmp.gpg` quarantine queue awaiting signature
  verification.
