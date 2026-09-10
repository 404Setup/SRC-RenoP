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

サーバーが公開する 15 種類の設定にそれぞれ独立したページを用意します。デスクトップではフォームの横に分類を表示し、
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

選択中の分類、未保存の印、補足テキストにはライト・ダーク両モードで共通のテーマ色を使用します。

## ドメインの読み取りと更新

- **読み取り**: `GET /api/settings/domain/:name`
- **更新**: `PUT /api/settings/domain/:name`
- **動作**: スキーマは `:name` ごとに異なります。不明なフィールドと不正値は拒否されます。ホスト、ポート、
  TLS、データベース、一部ランタイム設定の変更には再起動が必要な場合があります。

**外部ログイン**：`GET /api/settings/oauth-providers` は組み込みの GitHub 項目、秘匿化したクライアント、プリセットを返します。`PUT /api/settings/oauth-providers` は `providers` 配列を必須とし、両方の設定を原子的に保存します。GitHub と最大 32 件の他のクライアントに対応し、リクエスト上限は 128 KiB です。旧クライアントとの互換性のため GitHub の省略は設定を保持します。無効化して `clear_client_secret` で資格情報を削除してください。空の配列は他のクライアントのみを削除します。互換エンドポイント `GET /api/settings/github-oauth` と `PUT /api/settings/github-oauth` は同じ GitHub 設定を管理します。

[OAuth](../security/oauth-login.md)

## リポジトリ設定

通常は `/api/settings/repositories` を使用します。Maven プレフィックス付きルートは互換性のため残ります。

変更はデータベースへのコミット後に実行中の設定へ反映されます。最後のリポジトリを削除した状態も再起動後に保持されます。
旧 YAML は初回移行の入力としてのみ使用します。

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

[法的文書と Cookie の設定](../configuration/legal.md)

設定ページ、プロバイダー編集、関連フィールドはキャンセル可能な遷移を使い、動きを減らす設定に従います。未設定の一覧には共通の通知スタイルを使います。閉じた選択欄はメニューや文書のリスナーを保持せず、開いたときに作成します。

組み込みリソースは実行ファイルからストリーム送信します。RenoP は種類、サイズ、ETag を保持し、各バンドルと圧縮版の追加コピーを Go ヒープに保持しません。圧縮形式の交渉と条件付きリクエストは維持されます。

[セキュリティ認証](../security/captcha.md)
