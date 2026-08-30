---
title: API de cryptographie GPG
order: 11
category: Référence API
description: Gestion des clés publiques OpenPGP et suivi des validations de signatures
---

# API de cryptographie GPG

## Lister les clés GPG du compte

- **Chemin** : `GET /api/auth/profile/gpg`
- **Authentification** : requise.

### Réponse JSON

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

## Enregistrer une clé publique

- **Chemin** : `POST /api/auth/profile/gpg`
- **Authentification** : requise.
- **Corps JSON** :
  ```json
  {
    "public_key_armored": "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n-----END PGP PUBLIC KEY BLOCK-----"
  }
  ```
- **Réponse** : `200 OK` avec les métadonnées analysées de la clé.

---

## Lister les publications en quarantaine

- **Chemin** : `GET /api/auth/profile/gpg/releases`
- **Usage** : liste les artefacts conservés dans `.renop.tmp.gpg` en attente de leur signature détachée, de la
  validation
  de la clé ou de la publication finale.
