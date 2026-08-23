---
title: 概要
order: 1
category: はじめに
description: RenoP の概要、サポートプロトコルおよび基本機能
---

# RenoP の概要

RenoP は、セルフホスト型のマルチプロトコル・パッケージおよびアーティファクトリポジトリサーバーです。Go 言語で開発され、Web
管理 UI が組み込まれており、軽量で外部依存のないプライベートリポジトリ環境を提供します。

## サポートプロトコルとエコシステム

- **Maven / Gradle**: Release、Snapshot、Private リポジトリに対応。標準的な Maven ディレクトリ構造に準拠し、Javadoc
  のオンライン閲覧および GPG 署名検証をサポートします。
- **Cargo (Rust)**: Cargo スパースインデックス (Sparse Index) プロトコル、Crate の公開・ダウンロード・検索・yank、crates.io
  のプロキシミラー、Cargodoc オンラインドキュメント閲覧に対応しています。
- **Docker / OCI レジストリ**: OCI Distribution Spec v2 および Docker Registry v2 仕様に準拠し、マルチアーキテクチャマニフェスト、チャンク
  Blob アップロード、上流レジストリミラーに対応します。

## ストレージおよびデータベース

- **ストレージバックエンド**: ローカルファイルシステムまたは S3 互換オブジェクトストレージ (AWS S3, MinIO, Cloudflare
  R2, 各種クラウド OSS) をサポート。
- **データベース**: 組み込み SQLite を標準搭載し、外部 MySQL 8.0+ および PostgreSQL にも対応しています。

## 主な機能

| 機能                       | 説明                                                                                                             |
|:---------------------------|:-----------------------------------------------------------------------------------------------------------------|
| **単一バイナリ**           | 外部依存なしで即座に起動可能。Web UI もバイナリに内包されています                                                |
| **上流ミラーリング**       | Maven, Cargo, Docker の透過的プロキシとローカルキャッシュ（TTL および除外ルール設定可能）                        |
| **きめ細かなアクセス制御** | ロールベースアクセス制御 (RBAC)、リポジトリ単位の権限、パーソナルアクセストークン (PAT)                          |
| **システムサービス管理**   | 内置の `--install` および `--uninstall` コマンドで Windows サービス、systemd、OpenRC、LaunchDaemons、rc.d に対応 |
| **セキュリティ**           | Detached OpenPGP 署名検証、スライディングウィンドウによるレート制限、異常 IP の自動ブロック                      |

## ナビゲーション

- [インストールガイド](./install.md) — バイナリダウンロードとビルド手順
- [クイックスタート](./quickstart.md) — 初期起動、管理者パスワードとデフォルトエンドポイント
- [システムアーキテクチャ](./architecture.md) — 内部設計と処理フロー
- [設定概要](../configuration/overview.md) — 構成ファイルと環境変数
- [Maven & Gradle](../guides/maven-client.md) — クライアント設定
- [Cargo レジストリ](../guides/cargo-registry.md) — Rust / Cargo 設定
- [Docker レジストリ](../guides/docker-registry.md) — Docker / Podman 設定
