---
title: 設定 API
order: 8
category: API リファレンス
description: ドメイン別サービス設定、リポジトリ管理、インデックス再構築
---

# 設定 API

設定ルートには管理者、または操作に応じて `admin:settings` や `admin:repositories` を持つ API Token が
必要です。`proto/api/v1/api.proto` で定義されたレスポンスは protobuf を使用します。

## 設定ドメインの取得

- **パス**: `GET /api/settings/domains`
- **レスポンス**: `server`、`proxy`、`storage`、`updater`、`index` など、サーバーが対応する安定名です。

## ドメインの読み取りと更新

- **読み取り**: `GET /api/settings/domain/:name`
- **更新**: `PUT /api/settings/domain/:name`
- **動作**: スキーマは `:name` ごとに異なります。不明なフィールドと不正値は拒否されます。ホスト、ポート、
  TLS、データベース、一部ランタイム設定の変更には再起動が必要な場合があります。
- **GitHub OAuth**: `GET /api/settings/github-oauth` はマスク済み状態を返し、
  `PUT /api/settings/github-oauth` は Client ID と書き込み専用 Secret を更新します。

**その他の OAuth プロバイダー**：`GET /api/settings/oauth-providers` はシークレットを伏せたクライアントとプリセットを返し、`PUT /api/settings/oauth-providers` は一覧を置き換えます。`providers` 配列は必須で、明示的な空配列は設定済みクライアントをすべて削除します。クライアントは最大 32 件、本文は最大 128 KiB です。認証情報、プリセット、アカウント連携は[外部サービスでログイン](../security/oauth-login.md)を参照してください。

## リポジトリ設定

通常は `/api/settings/repositories` を使用します。Maven プレフィックス付きルートは互換性のため残ります。

### リポジトリ一覧

- **パス**: `GET /api/settings/repositories`
- **別名**: `GET /api/settings/maven/repositories`

### 作成、更新、削除、移行

- **作成または更新**: `PUT /api/settings/repositories/:name`
- **削除**: `DELETE /api/settings/repositories/:name`
- **Maven/files 移行**: `POST /api/settings/repositories/:name/migrate/:target`。`:target` は `maven` または
  `files` です。保存済みオブジェクトは移動せず、Maven に戻す際にカタログを再構築します。

## 検索インデックスの再構築

- **パス**: `POST /api/settings/index/rebuild`
- **動作**: 統合可能なバックグラウンド再構築を投入し、同じ処理を並行起動しません。

## 公開ドメインの予約期間

`GET /api/settings/maven-domains` と `PUT /api/settings/maven-domains` は JSON を使用します。
設定一覧には `maven_domains` が含まれます。既定値は次のとおりです。

```json
{"release_value":2,"release_unit":"year"}
```

`release_value` は 1–100 の整数、`release_unit` は `month` または `year` で、UTC の暦に従って計算します。
設定ファイルの `maven_domains` に同じフィールドを保存します。保存後は新しい安全ロックに適用され、
既存の解放日や自主閉鎖の独立した 31 日間の予約期間は変更しません。
[Maven ドメイン状態](maven.md)を参照してください。
