---
title: リポジトリとミラー
order: 2
category: 設定
description: エンジン、可視性、上流ミラー、移行、S3 ストレージ
---

# リポジトリとミラー

定義は `repositories.yaml` にあり、`RENOP_REPOSITORIES` で上書きできます。管理 UI も同じ検証済み設定を
編集します。名前は不変の小文字 slug で、URL の最初の segment です。

## 設定例

```yaml
repositories:
  releases:
    name: releases
    format: maven
    visibility: PUBLIC
    allow_redeployment: false
    require_gpg_signature: true
    publication_review: every_version
    download_statistics: true
    mirrors: []
  crates:
    name: crates
    format: cargo
    visibility: PUBLIC
    mirrors: []
  containers:
    name: containers
    format: docker
    visibility: PRIVATE
    allow_redeployment: false
    mirrors: []
```

## リポジトリ項目

| 項目 | 既定 | 説明 |
|:-----|:-----|:-----|
| `name` | 必須 | 不変 slug と URL prefix |
| `format` | `maven` | `maven`、`maven-classic`、`files`、`npm`、`cargo`、`docker` |
| `visibility` | `PUBLIC` | `PUBLIC`、`HIDDEN`、`PRIVATE` |
| `allow_redeployment` | `false` | 対応形式で Maven 再公開または files/Docker 上書き |
| `require_gpg_signature` | `false` | Maven 公開時の OpenPGP 分離署名検証 |
| `publication_review` | `off` | Maven/npm/Cargo/Docker 審査方針: `off`、`new_packages`、`every_version` |
| `download_statistics` | エンジン既定 | Maven/npm/Cargo/Docker は有効、`files` は明示的に有効化 |
| `mirrors` | `[]` | 順序付き上流定義 |
| `s3` | 省略 | リポジトリ固有 S3 storage |

npm と Docker の `new_packages` は、名前を予約する前に明示的な作成 request を審査します。`every_version` は
その後の各 version または manifest も審査します。Maven と Cargo には空 package の作成段階がないため、
`new_packages` は最初の公開を審査します。mirror import はすべての engine で審査対象外です。

`maven-classic` は画面レイアウトだけを変え、Maven の公開規則を維持します。`files` は非構造化で、
チェックサムや POM の生成、署名検証を行いません。Maven と `files` の相互移行ではオブジェクトを移動せず、
Maven へ戻す際にカタログと保存済みの方針を復元します。移行前のダウンロード統計設定も維持されます。

公開審査は Maven と npm に対応します。Maven では `allow_redeployment` を `false` に固定し、npm では不変
バージョンと dist-tag のトランザクションを維持します。ローカルファイルはリポジトリモデレーターまたはシステム
管理者の承認まで非公開となり、ミラーは審査されません。保留中の審査があるリポジトリは設定変更、削除、
エンジン移行ができません。

`npm` repository は publication 前の package 予約、不変 SemVer、dist-tag、scoped private package、L0-L4 team、
完全名または `@scope/*` 規則の mirror を提供します。

### 可視性

- **PUBLIC**: 匿名の読み取りと発見を許可します。
- **HIDDEN**: 匿名ユーザーや閲覧権限のないユーザーの一覧とプロフィールの所属情報には表示されません。
  管理者と明示的なリポジトリ閲覧権限を持つユーザーには表示されます。既知の正確なファイルパスは読み取れます。
- **PRIVATE**: 読み取り、一覧、書き込みに明示権限が必要です。非公開 Docker image は L0-L4 も確認します。

## 上流ミラー

ローカルにない object は有効ミラーから stream し、本文全体を buffer せず保存できます。Cargo と Docker は
適用対象の上流名が存在する場合、ローカル作成を拒否します。

```yaml
mirrors:
  - name: "central"
    url: "https://repo1.maven.org/maven2"
    persist: true
    cache_ttl_secs: 86400
    negative_cache: true
    timeout_secs: 30
    proxy: ""
    allow_artifacts: []
    deny_artifacts: []
```

| 項目 | 既定 | 説明 |
|:-----|:-----|:-----|
| `name` | 必須 | リポジトリ内で一意の名前 |
| `url` | 必須 | 上流 base URL |
| `persist` | `true` | 成功レスポンスを保存 |
| `cache_ttl_secs` | `86400` | positive cache lifetime |
| `negative_cache` | `true` | 対応する上流 miss を cache |
| `timeout_secs` | `30` | 上流要求 timeout |
| `proxy` | `""` | 全体 route、`direct`、または名前付き proxy |
| `allow_artifacts` | `[]` | format-aware allow rule |
| `deny_artifacts` | `[]` | 優先される deny rule |

資格情報は構造化 authorization 項目に置き、`url` に埋め込まないでください。

## S3 互換ストレージ

各リポジトリは Disk または独立 S3 を使用できます。storage/engine 変更は repository gate が upload、delete、
GPG commit、mirror write と直列化します。

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

MinIO は通常 `force_path_style` を必要とします。`redirect_downloads` 有効時は認可後に短期署名 URL へ
redirect し、無効時は RenoP が stream します。
