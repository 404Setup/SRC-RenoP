---
title: ストレージ構成
order: 3
category: デプロイ
description: リポジトリ単位のローカルファイルと S3 互換 object storage
---

# ストレージ構成

RenoP はローカル disk と S3 互換 object service に対応します。各リポジトリが backend を選択し、変更時は
repository gate が進行中操作と直列化します。

## ローカルファイルシステム

ルートは `config.yaml` の `storage_path` で、既定は `storage` です。

### 配置

- **Maven/files**: `{storage_path}/{repo}/{path}`
- **Cargo**: index と archive はリポジトリディレクトリ内で分離
- **Docker**: blob、manifest、reference を分離し image 単位で検証

物理名は内部実装です。ディレクトリを直接操作せず protocol API を使用してください。

### 書き込み信頼性

- upload は上限付き一時ファイルを使い、確定前に size、hash、policy を検証します。
- filesystem が対応する場合、最終公開は atomic です。
- mirror commit、delete、migration、GPG publication は backend 変更と同期します。

---

## S3 互換 object storage

S3 は managed object storage に適します。multi-node には外部 DB と RenoP の保証に沿った coordination も
必要で、S3 だけで単一 process が cluster になるわけではありません。

### Provider

- **AWS S3**
- **MinIO**
- **Cloudflare R2**
- 必要な S3 API を実装する service

### 例 (`repositories.yaml`)

```yaml
repositories:
  releases:
    name: releases
    s3:
      enabled: true
      endpoint: "https://minio.internal:9000"
      bucket: "renop-storage"
      key_prefix: "releases/"
      region: "us-east-1"
      access_key_id: "ACCESS_KEY"
      secret_access_key: "SECRET_KEY"
      force_path_style: true
      redirect_downloads: false
```

bucket は事前作成し、credentials に `key_prefix` 下の read/write/list/delete を許可します。TLS と secret
manager を使い、Git リポジトリへ鍵を commit しないでください。

### ダウンロード方式

- **Proxy streaming (`redirect_downloads: false`)**: RenoP が認可後に S3 から client へ stream します。
  bucket を非公開に保ち、S3 URL を隠せます。
- **Direct redirect (`redirect_downloads: true`)**: 認可後に短期 presigned URL への `302 Found` を返し、
  RenoP の帯域使用を減らします。
