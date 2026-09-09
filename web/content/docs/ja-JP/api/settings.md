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

## ブラウザーの設定ページ

サーバーが公開する 14 種類の設定にそれぞれ独立したページを用意します。デスクトップではフォームの横に分類を表示し、
小さい画面では分類セレクターを使います。前へ・次へも同じ順序で移動します。ページを開くとその設定だけを読み込み、
サービス全体の設定を同時には取得しません。

分類や言語を切り替えても、ページごとの未保存の下書きを保持します。保存は現在のページだけを更新し、送信中は編集と
ページ切り替えを一時的に停止します。失敗時は下書きを維持し、破棄は確認後に現在のページを再取得します。
未保存の変更は分類ナビゲーションに表示します。ブラウザーの再読み込みや終了時には警告しますが、下書きはメモリーだけに
保持され、ログアウトやアカウント変更時に消去します。保存済みの書き込み専用資格情報は非表示のままで、入力した秘密情報は
保存成功後に下書きから消去します。

GPG は引き続きサービス設定に含まれます。チーム上限、公開クォータ、登録、キャッシュ、メール、OAuth プロバイダー、
公開ドメインの安全設定は独立したページで既存の JSON API を利用します。ラベルと説明を入力に関連付け、ページ移動時は
新しい見出しへキーボードフォーカスを移します。

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
