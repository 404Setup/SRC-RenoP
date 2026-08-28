---
title: システムアーキテクチャ
order: 4
category: はじめに
description: モジュール、認可、streaming storage、非同期処理
---

# システムアーキテクチャ

RenoP は transport、package protocol、認可、永続化、background maintenance の境界を持つ単一 Go process
です。埋め込み frontend も外部 client と同じ上限付き API を呼びます。

## モジュール境界

```text
Browser and package clients
        |
HTTP routing, rate limits, authentication, API-token policy
        |
Maven | npm | Cargo | Docker | Files | Management services
        |
Repository gate and publication workflows
        |
Disk or S3 storage          SQL database
        |                       |
File index and mirrors      Identity, teams, audit, messages
```

- `internal/api` と middleware は一般 HTTP contract、search、anomaly、credential boundary を所有します。
- format service は Maven domain/catalog、npm packument、Cargo Sparse Index、Docker Distribution v2、doc viewer を所有します。
- database layer は SQLite、MySQL、PostgreSQL の dialect-aware transaction を提供します。
- Disk/S3 は巨大 body を stream し、file index は上限付き metadata traversal を提供します。

## リクエストと作業 pipeline

### Streaming と consistency

upload/download は client と Disk/S3 間を stream します。hash、Brotli/ZIP extraction、mirror cache、GPG は
bounded reader と一時 file を使います。striped repository gate が storage/engine 変更と upload、delete、
mirror commit、publication の race を防ぎます。

### 認証と認可

browser session は cookie-only、Basic は標準 package protocol 専用です。Bearer API Token の scope と対象制限は
毎回、現在の repository permission と L0-L4 membership と交差します。不変 user ID が username 変更後も
所有権を保ちます。

### 非同期処理

process-wide non-reentrant scheduler が snapshot、cleanup、index、download counter、update check を統合します。
順序が必要な audit、GPG、Token mutation、file watch は専用 serial worker を維持します。durable result は
message center、一時 progress は UI state または Toast に送ります。
