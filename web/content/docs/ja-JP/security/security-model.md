---
title: セキュリティと権限
order: 1
category: セキュリティ
description: Credential boundary、repository permission、package team、defense in depth
---

# セキュリティと権限

RenoP は credential type、API Token capability、account role、repository visibility、対象 team を組み合わせて
認可します。所有 account が失った権限を credential が保持することはありません。

## Account / system role

| Role または permission | 効果 |
|:-----------------------|:-----|
| Anonymous | `PUBLIC` と `HIDDEN` の既知 exact path を読む |
| `base` | 暗黙 repository write のない認証 account |
| `canview:{repo}` / `canview:*` | 指定または全 repository を private 含め read |
| `canupdate:{repo}` / `canupdate:*` | package/domain policy の範囲で publish |
| `showing` | legacy compatibility。hidden は user catalog に表示しない |
| `allview` / `proview` | legacy global private-read alias |
| `manager` / `admin` | user、repository、settings、audit、update、全 team の super administrator |

system admin は global です。package team L0-L4 は通常 collaboration の権限として分離します。admin operation
は audit され、表示 team member を暗黙作成しません。

## Repository / team layer

- visibility は discovery/read boundary の `PUBLIC`、unlisted `HIDDEN`、authorized `PRIVATE` です。
- repository permission は Cargo/Docker package 作成や Maven domain 検証を自動で行いません。
- Cargo/Docker team は L0 read、L1 publish、L2 lifecycle/metadata、L3 member、L4 owner です。
- Maven team は検証済み global domain に属し、全 Maven repository で有効です。
- private Docker image は public L0 を暗黙付与せず、blob も読める image に制限します。

## Credential transport

- **Browser session**: HttpOnly `renop_session`。private security と Token management に必要。
- **Basic**: username + password/API Token。標準 package protocol 専用。
- **Bearer API Token**: API/package automation の capability と exact target policy。
- **Docker Bearer**: source credential と image が許可した action だけの短期 token。

`Authorization: Session`、URL session secret、query credential は拒否します。Token scope/target は現在の
account authorization と常に交差します。

## Defense in depth

- password/recovery code は salted one-way verification、API Token plaintext は非永続です。
- session は idle expiry と device revoke に対応し、recovery は既存 session を atomic に失効します。
- rate limit、progressive ban、active bound、trusted proxy validation が network を保護します。
- upload、archive、mirror、update は bounded streaming、path validation、hash、temp storage を使います。
- audit と durable message は security result を記録し、neutral notification では operator を公開しません。
