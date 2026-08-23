---
title: ストレージ構成
order: 3
category: デプロイと運用
description: ローカルファイルシステムと S3 互換オブジェクトストレージ
---

# ストレージ構成

- **ローカルストレージ**: `storage_path` で指定されたパスにアトミック書き込みで安全に保存。
- **S3 互換ストレージ**: AWS S3、MinIO、Cloudflare R2 等に対応。プロキシ転送または 302 署名付き URL リダイレクト
  (`redirect_downloads: true`) を選択可能。
