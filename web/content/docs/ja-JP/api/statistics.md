---
title: ダウンロード統計 API
order: 14
category: API リファレンス
description: 上限付きダウンロード集計、階層クエリ、リポジトリ設定、API Token 要件
---

# ダウンロード統計 API

RenoP はリクエストごとの行を保存せず、成功したパッケージダウンロードを集約します。カウンターには回数、
論理バイト数、最終更新時刻が含まれます。ユーザー帰属はアカウントの不変 ID に結び付くため、username を
変更しても履歴は分割されません。

Maven、npm、Cargo、Docker リポジトリは既定で集計します。非構造化 `files` エンジンは明示的な有効化が必要です。
checksum、分離署名、Maven metadata、Javadoc companion は除外します。`HEAD`、`304`、失敗したリクエスト、
先頭以外の range segment は集計しません。Docker は各 blob ではなく manifest 応答時に 1 pull を記録します。

## アカウントの照会

`GET /api/statistics` は API Token 所有者の統計を返します。`GET /api/statistics/users/:username` も同じ
アカウント境界を使用し、別アカウントの照会にはシステム管理者 Token が必要です。

両 route は `statistics:read` を持つ Bearer API Token 専用です。ブラウザ session cookie と Basic 認証は
拒否されます。照会前にメモリ上のカウンターを flush するため、現在の server process が受理済みの
ダウンロードも成功レスポンスに含まれます。

## システムの照会

`GET /api/statistics/system` にはシステム管理者アカウントと `admin:statistics` scope が必要です。
`user`、`repository`、`namespace`、`package`、`version` で grouping できます。アカウント route は `user`
以外の grouping を利用できます。

任意の完全一致 filter は `username`（system のみ）、`repository`、`format`、`namespace`、`package`、
`version` です。ページネーションでは 1〜100 の `limit` と、0〜1,000,000 の `offset` を使用します。
各ページは完全なフィルター結果の `count`、`bytes` と、グループの総数も返します。

## リポジトリ設定

管理者は `GET /api/settings/repositories/download-statistics` で有効状態を読み、
`PUT /api/settings/repositories/:name/download-statistics` で変更します。JSON body は
`{"enabled": true}` または `{"enabled": false}` です。

`DELETE /api/settings/repositories/:name/download-statistics` は保存済み・書き込み待ちの全カウンターを
完全に消去します。Docker では image page の互換 pull count もリセットします。リポジトリ削除時には
対応する統計も自動削除されます。

完全なレスポンス schema と制限は `web/assets/openapi.yaml` にあります。
