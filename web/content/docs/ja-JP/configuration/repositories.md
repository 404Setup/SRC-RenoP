---
title: リポジトリとミラー
order: 2
category: 設定
description: repositories.yaml の設定、公開範囲、上流プロキシミラー、S3 ストレージ
---

# リポジトリとミラー設定

リポジトリ設定は `repositories.yaml` で管理します。

## 設定例

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false

  snapshots:
    name: snapshots
    visibility: PUBLIC
    allow_redeployment: true
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false

  private:
    name: private
    visibility: PRIVATE
    allow_redeployment: false
    require_gpg_signature: false
    mirrors: []
    s3:
      enabled: false
```

### 公開範囲 (Visibility)

- **PUBLIC**: 認証不要で誰でもダウンロード可能。
- **HIDDEN**: URL を直接知っているユーザーは取得可能だが、一覧には表示されない。
- **PRIVATE**: 認証および対象リポジトリのアクセス権限が必要。

## 上流ミラー設定 (`mirrors`)

```yaml
mirrors:
  - name: "maven-central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

## S3 ストレージ設定 (`s3`)

```yaml
s3:
  enabled: true
  endpoint: "https://s3.us-east-1.amazonaws.com"
  bucket: "my-renop-bucket"
  key_prefix: "releases/"
  region: "us-east-1"
  access_key_id: "YOUR_ACCESS_KEY"
  secret_access_key: "YOUR_SECRET_KEY"
  force_path_style: false
  redirect_downloads: false
```
