---
title: Token и подписи GPG
order: 2
category: Безопасность
description: Точные machine credentials, recovery material и проверка OpenPGP publications
---

# Token и подписи GPG

RenoP разделяет browser sessions, API Token, passwords, recovery material и signing keys. Для них действуют разные
правила storage, transport и revocation.

## API Token и recovery material

API Token использует 256 random bits и префикс `rnp_pat_`. Secret показывается один раз; хранится только SHA-256 lookup
digest. Token имеет private label, scopes, необязательные exact repository/package/team/domain targets и expiration.
Аккаунт имеет до 50 Token, Token — до 128 targets.

Используйте least privilege и короткий lifetime. Операцию должны разрешать Token policy и текущие права аккаунта.
Revocation немедленно очищает auth cache. Legacy plaintext secrets мигрируют в hashed compatibility credentials.

Browser session — cookie-only, Basic — package-protocol-only, automation отправляет
`Authorization: Bearer <token>`. Query credentials игнорируются или отклоняются.

Recovery codes отдельны. Набор содержит 12 one-time codes с Argon2id verifiers. Четыре разных неиспользованных кода
атомарно сбрасывают password, расходуются, отзывают sessions и включают password login. Храните их offline и заменяйте
после использования или подозрения на утечку.

---

## Проверка отделённых OpenPGP подписей

Maven repository может потребовать `.asc` до видимости artifact. Пользователь регистрирует public key; private key не
попадает в RenoP.

### Включение проверки

```yaml
repositories:
  releases:
    name: releases
    format: maven
    require_gpg_signature: true
```

### Publication flow

1. RenoP stream artifact в `.renop.tmp.gpg` и создаёт bounded pending release.
2. Соответствующий `.asc` может прийти до или после artifact в пределах deadline.
3. RenoP разрешает unambiguous registered fingerprint и повторно проверяет signature, uploader и policy под gate.
4. Валидная пара atomic commit, verified metadata сохраняется для UI.
5. Invalid, missing, expired, deleted или unauthorized release завершается со stable reason.

Key server URLs используют HTTPS и `server.gpg.key_servers`. Requests следуют proxy policy, используют bounded clients и
никогда не отправляют private key.
