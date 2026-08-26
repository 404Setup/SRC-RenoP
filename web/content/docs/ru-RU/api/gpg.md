---
title: API криптографии GPG
order: 11
category: Справочник API
description: Управление открытыми ключами OpenPGP и состоянием проверки подписей
---

# API криптографии GPG

## 1. Список ключей GPG аккаунта

- **Путь**: `GET /api/auth/profile/gpg`
- **Аутентификация**: обязательна.

### Ответ JSON

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

## 2. Регистрация открытого ключа

- **Путь**: `POST /api/auth/profile/gpg`
- **Аутентификация**: обязательна.
- **Тело JSON**:
  ```json
  {
    "public_key_armored": "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n-----END PGP PUBLIC KEY BLOCK-----"
  }
  ```
- **Ответ**: `200 OK` с разобранными метаданными ключа.

---

## 3. Список публикаций в карантине

- **Путь**: `GET /api/auth/profile/gpg/releases`
- **Назначение**: возвращает артефакты в `.renop.tmp.gpg`, ожидающие отделённую подпись, проверку ключа или окончательную
  публикацию.
