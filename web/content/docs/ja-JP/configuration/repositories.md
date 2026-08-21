---
title: リポジトリとミラー
order: 2
category: 設定
description: repositories.yaml — 可視性、ミラー、S3
---

# リポジトリとミラー

ファイル: `repositories.yaml`（`RENOP_REPOSITORIES` で上書き可）。

デフォルトリポジトリ:

| 名前        | 用途                            |
|-------------|---------------------------------|
| `releases`  | リリース（通常 PUBLIC）         |
| `snapshots` | スナップショット（通常 PUBLIC） |
| `private`   | プライベート（PRIVATE）         |

`repositories:` 配下に名前でキー付け。

## リポジトリフィールド

```yaml
repositories:
  releases:
    name: releases
    visibility: PUBLIC          # PUBLIC | HIDDEN | PRIVATE
    allow_redeployment: false
    mirrors: [ ]
    s3:
      enabled: false
      endpoint: ""
      bucket: ""
      key_prefix: ""
      region: auto
      access_key_id: ""
      secret_access_key: ""
      force_path_style: true
      redirect_downloads: false
```

| フィールド              | 説明                                                                                       |
|-------------------------|--------------------------------------------------------------------------------------------|
| `name`                  | リポジトリ ID（パスセグメント: `http://host:port/{name}/…`）                               |
| `visibility`            | `PUBLIC` 匿名読み取り可、`HIDDEN` 一覧制限あり、`PRIVATE` 読み取り権限必要                 |
| `allow_redeployment`    | 既存成果物パスの上書き許可（デフォルト: releases/private は `false`、snapshots は `true`） |
| `require_gpg_signature` | `.jar`、`.pom`、`.module` に分離 GPG 署名を必須とし、検証完了まで公開を待つか              |
| `mirrors`               | 上流 Maven プロキシ（任意）                                                                |
| `s3`                    | このリポジトリの S3 互換バックエンド（任意）                                               |

各リポジトリ配下の Maven レイアウトは標準: `group/artifact/version/file`。

## ミラー

未ヒット時、ミラーは上流から取得してキャッシュできます。

| フィールド        | 説明                                                                          |
|-------------------|-------------------------------------------------------------------------------|
| `name`            | 表示名/設定名                                                                 |
| `url`             | 上流ベース URL                                                                |
| `persist`         | キャッシュした成果物をストレージに永続化                                      |
| `cache_ttl_secs`  | ポジティブキャッシュ TTL（秒）                                                |
| `negative_cache`  | 「見つからない」レスポンスをキャッシュ                                        |
| `timeout_secs`    | 上流リクエストタイムアウト                                                    |
| `authorization`   | 認証情報（任意）: `method`、`login`、`password`                               |
| `proxy`           | 空欄はグローバル設定を継承、`direct` は直接接続、名前は設定済みプロキシを選択 |
| `enabled_date`    | 有効化日時文字列（任意）                                                      |
| `allow_artifacts` | 設定時、一致する `group` または `group:artifact` パターンのみプロキシ         |
| `deny_artifacts`  | 設定時、一致する座標をブロック（allow と併用不可）                            |

認証方式の一般的な使用例: `BASIC` / ユーザー名・パスワード、または `Bearer` / トークン。

ミラーのプロキシ資格情報は `repositories.yaml` に保存されません。グローバル `proxy` 設定で名前付きプロキシを 定義し、ミラーの単一
`proxy` セレクターを使用してください。

## 可視性 vs 権限

| 可視性  | 匿名読み取り                                             | 備考                                                                   |
|---------|----------------------------------------------------------|------------------------------------------------------------------------|
| PUBLIC  | 可                                                       | オープンリポジトリ                                                     |
| HIDDEN  | ファイル取得は可能な場合あり、ルート一覧は追加ロール必要 |                                                                        |
| PRIVATE | 不可                                                     | `canview` / `allview` / `proview`、書き込み権限、または manager が必要 |

書き込みには常に `canupdate`（または manager）が必要です。詳細: [認証](../api/authentication.md)。

## S3 互換ストレージ

`s3.enabled` が true の場合、そのリポジトリの成果物は指定されたバケットに保存されます。主なフィールド:

| フィールド                            | 説明                                                      |
|---------------------------------------|-----------------------------------------------------------|
| `endpoint`                            | S3 API エンドポイント                                     |
| `bucket`                              | バケット名                                                |
| `key_prefix`                          | バケット内のオブジェクトキープレフィックス（任意）        |
| `region`                              | リージョン（または `auto`）                               |
| `access_key_id` / `secret_access_key` | 認証情報                                                  |
| `force_path_style`                    | パススタイル URL（MinIO で一般的）                        |
| `redirect_downloads`                  | サポート時にクライアントをオブジェクト URL へリダイレクト |

`key_prefix` が空の場合、RenoP
はレガシーオブジェクトレイアウトを保持します。既に成果物が含まれるリポジトリにプレフィックスを追加または変更する前に、既存オブジェクトを新しいプレフィックスに移動してください。RenoP
は自動的に移行しません。

## 関連項目

- [設定概要](./overview.md)
- [ストレージ API](../api/storage.md)
- [GPG 署名](../api/gpg.md)
- [Maven クライアント](../getting-started/maven-client.md)
