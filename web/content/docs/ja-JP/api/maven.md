---
title: Maven レジストリ API
order: 4
category: API リファレンス
description: 検証済み公開ドメイン、チーム、成果物カタログ、Maven クライアントアクセス
---

# Maven レジストリ API

RenoP の Maven リポジトリは、検証済みの逆ドメイン名前空間を使用します。公開者はアカウントメニューで
ドメインを一度だけ予約し、許可されたすべての Maven リポジトリで再利用できます。Maven 2 のパス、
メタデータ、分離署名、チェックサムは Maven と Gradle に互換です。

## ドメイン検証

`POST /api/maven/domains` でドメインを作成します。RenoP は高エントロピーのコードと証明先を返します。

- DNS 名前空間は登録ルートに TXT を設定します。すべての TXT 値を読み、完全一致のみを受理します。
- `io.github.<account>` は公開 GitHub ユーザーの Bio または公開 Organization の Description を使用します。
- `io.gitlab.<account>` は公開 GitLab ユーザーの Bio または公開 Group の Description を使用します。

`POST /api/maven/domains/:domain/verify` で外部検証を開始します。各ドメインにつき 5 秒に 1 回までです。
システム管理者は `/verify/force` を利用でき、この操作は監査ログに記録されます。

検証済みドメインとチームはインスタンス全体で共有されます。別の Maven リポジトリでも再検証や再招待は
不要です。

## ドメイン権限

Maven チームはリポジトリや個別成果物ではなく、グローバルドメインに属します。

- L0: 公開内容の読み取り
- L1: 成果物の公開
- L2: バージョンと説明の管理
- L3: メンバーの招待と管理
- L4: ドメインの所有と移譲

招待要求は 1～20 ユーザー名を受け取ります。管理者以外の追加はメッセージセンターの招待になります。
移譲時も L4 所有者は常に 1 人で、所有者は退出前に所有権を移譲する必要があります。

## 成果物カタログ

`GET /api/maven/repositories/:repo/domains` は、そのリポジトリに成果物があるドメインを返します。
`GET /api/maven/repositories/:repo/packages` はページング検索を提供します。
`GET /api/maven/repositories/:repo/package?group=...&artifact=...` は成果物とバージョンを返します。L2 は関連
JSON API で説明の更新と完全なバージョンの削除を行えます。

旧リポジトリはアップグレード時に索引化されます。移行ドメインは検証済みですが、メンバーは自動追加され
ません。設定済み Maven ミラーは不足成果物を引き続き解決します。

## レイアウトとファイルリポジトリ

既定 UI はドメインカタログです。管理者は従来のファイルツリーへ切り替え、後から戻せます。変更されるのは
表示だけで、任意パスは拒否され、公開には検証済みドメインと正しい Maven パスが必要です。

独立した `files` 形式は非構造化データ向けです。上書き、削除、S3、ミラーを利用できますが、チェックサム、
POM、OpenPGP 検証は生成または実行しません。

## Maven / Gradle クライアントアクセス

読み取りと公開は `/{repo}/{maven-path}` を使用します。パスワード、または `repository:read` や
`repository:publish` を持つ API Token で認証します。可視性が読み取りを制御し、検証済みドメインと
アカウントの L0-L4 が変更操作を制御します。完全な仕様は `web/assets/openapi.yaml` にあります。
