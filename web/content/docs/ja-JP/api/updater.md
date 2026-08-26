---
title: インプレースアップデーター API
order: 13
category: API リファレンス
description: 更新確認、オンラインおよびオフラインインストール、再起動
---

# インプレースアップデーター API

更新操作には管理者セッション、または `admin:updates` を持つ API Token が必要です。エラーは JSON と
`X-Renop-Error-Code` で返され、内部パスや生のネットワークエラーを表示せずにローカライズできます。

## 状態を読む

`GET /api/updater/status` は protobuf `UpdateState` を返します。状態は `idle`、`checking`、`available`、
`downloading`、`ready_to_restart`、`error` のいずれかです。オンラインインストール中はこの API を
ポーリングして進行状況を取得します。

## 設定済みチャネルを確認する

`POST /api/updater/check?channel=release|nightly` は JSON `CheckResult` を返します。省略可能なクエリは
この要求に限ってチャネルを上書きします。結果には対象、SHA-256、サイズ、リリースノート、および現在の
版から対象版まで保持されている変更範囲が含まれます。

## オンラインインストールを開始する

`POST /api/updater/install` は、制限付きダウンロード、ダイジェスト検証、Brotli/ZIP 展開、バイナリ検証を
バックグラウンドで開始します。成功時は `{"status":"started"}` を返し、自動再起動は行いません。

ダウンロード進行は一時的な Toast であり、メッセージセンターには保存しません。確認結果と失敗結果は
管理者向け通知として保存されます。

## オフラインパッケージをインストールする

`POST /api/updater/upload` は multipart の `file` または `package` に、生の `.br` または従来の `.zip`
を受け取ります。大きなファイルは `purpose=updater` の分割アップロードを使用し、
`POST /api/upload/chunked/{upload_id}/complete` で完了します。

サーバーは容量を制限した一時領域へストリーム処理し、バイナリの対象プラットフォームを検証して
`ready_to_restart` を返します。失敗時に内部パスは返しません。

## 再起動する

`POST /api/updater/restart` は準備済みバイナリがあれば適用し、RenoP を再起動します。JSON
`{"status":"restarting"}` が届く前に接続が閉じる場合があります。公式 UI は再起動前に Toast を表示します。

## 安定したエラーコード

`X-Renop-Error-Code` は `forbidden`、`insufficient_space`、`missing_file`、`install_busy`、`invalid_package`、
`incompatible_binary`、`package_too_large`、`package_processing_failed`、`check_failed`、`notification_failed`、
`restart_failed` のいずれかです。
