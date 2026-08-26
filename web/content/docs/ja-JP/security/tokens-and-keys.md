---
title: Token と GPG 署名
order: 2
category: セキュリティ
description: 細粒度 machine credential、recovery material、OpenPGP publication verification
---

# Token と GPG 署名

RenoP は browser session、API Token、password、recovery material、artifact signing key を分離し、それぞれ異なる
storage、transport、revocation rule を適用します。

## API Token と recovery material

API Token は 256 random bits と `rnp_pat_` prefix を使います。secret は一度だけ表示し、SHA-256 lookup digest
だけを保存します。private label、scope、任意の exact repository/package/team/domain target、任意 expiry を持ちます。
account は最大 50 Token、Token は最大 128 target です。

least privilege と短い lifetime を使います。Token policy と account の現在の system/repository/domain/team
permission の両方が必要です。revocation は auth cache を即時 clear します。legacy plaintext は hashed
compatibility credential に移行します。

browser session は cookie-only、Basic は package-protocol-only、automation は
`Authorization: Bearer <token>` です。query credential は無視または拒否します。

recovery code は API Token と別です。12 個の one-time code を生成し、Argon2id verifier を保存します。異なる
未使用 4 code が password を atomic reset し、code 消費、session revoke、password login 再有効化を行います。
offline 保存し、使用後または漏えい疑い時は再生成してください。

---

## OpenPGP 分離署名検証

Maven repository は artifact 公開前に有効な `.asc` を必須にできます。user は public key を登録し、private
key を RenoP に渡しません。

### 検証の有効化

```yaml
repositories:
  releases:
    name: releases
    format: maven
    require_gpg_signature: true
```

### Publication flow

1. artifact を `.renop.tmp.gpg` へ stream し、bounded pending release を作成します。
2. 対応 `.asc` は deadline 内なら artifact 前後どちらでも受理します。
3. unambiguous registered fingerprint を解決し、signature、uploader、repository/domain policy を gate 内で再確認します。
4. 有効 pair を atomic commit し、verified metadata を UI 用に保存します。
5. invalid/missing/expired/deleted/unauthorized release は stable reason で失敗します。

key server は `server.gpg.key_servers` の HTTPS URL です。request は proxy policy と bounded client を使い、
private key を送信しません。
