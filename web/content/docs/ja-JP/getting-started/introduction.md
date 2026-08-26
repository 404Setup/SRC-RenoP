---
title: はじめに
order: 1
category: はじめに
description: 統合型セルフホスト package publication platform としての RenoP
---

# RenoP について

RenoP は統合型のセルフホスト package publication/distribution server です。Maven 専用 repository ではなく
private Central に近いモデルで、1 つの Go process に管理 UI、identity、team、verification、catalog、mirror、
storage、audit、update を統合します。

## 対応プロトコル

- **Maven / Gradle**: 検証済み global domain、modern catalog、classic layout 互換、Maven 2 path、mirror、
  Javadoc、OpenPGP 分離署名。
- **Cargo**: Sparse Index、明示的所有権、publication、search、yank/unyank、mirror、Cargodoc。
- **Docker / OCI**: Distribution v2、image 予約、private team、chunked blob、cross-repository mount、multi-arch、mirror。
- **Files**: mirror と上書きを備え、Maven metadata や署名 workflow を生成しない非構造化 storage。

## ストレージとデータベース

- **Storage**: streaming local Disk または repository 固有 S3-compatible object storage。
- **Database**: 既定の組み込み SQLite、外部 MySQL、PostgreSQL。
- **Consistency**: repository gate が upload、delete、mirror commit、GPG、engine/storage 変更を調整し、巨大 object
  全体を memory に読みません。

## 主な機能

| 機能 | 説明 |
|:-----|:-----|
| **単一サービス** | 別 application runtime なしで frontend と protocol API を内蔵 |
| **Global identity** | username 公開 profile と不変 internal user ID |
| **細粒度アクセス** | repository permission、L0-L4 team、対象/期限付き API Token |
| **検証済み公開** | Maven domain 所有権、上流名競合、任意 OpenPGP quarantine |
| **運用** | native service、scheduled task、durable audit/message、in-place update |
| **防御** | bounded streaming、rate limit、ban、trusted proxy、sandboxed viewer |

## ドキュメント案内

- [インストール](./install.md) — release package、platform、source build
- [クイックスタート](./quickstart.md) — 初回起動、管理者、repository 作成
- [アーキテクチャ](./architecture.md) — module、認可、storage、task
- [設定](../configuration/overview.md) — 検証済み設定と環境変数
- [Maven / Gradle](../guides/maven-client.md) — 検証 domain と JVM client
- [Cargo](../guides/cargo-registry.md) — Sparse registry と crate lifecycle
- [Docker / OCI](../guides/docker-registry.md) — image 予約、login、push、pull
