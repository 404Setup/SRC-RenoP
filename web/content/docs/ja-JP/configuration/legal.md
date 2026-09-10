---
title: 法的文書と Cookie の設定
order: 15
category: 設定
description: 規約ページ、ログイン時の同意、ブラウザー設定を構成する
---

# 法的文書と Cookie の設定

## 規約ページ

管理者は法的文書の設定ページでプライバシーポリシー、利用規約、法的情報を編集します。共通の Markdown エディターと安全なプレビューを使い、各文書は UTF-8 で最大 512 KiB です。空欄は仮の文章に戻るため、実際の文書に置き換えてください。

文書は `config.yaml` の `legal` に保存され、保存後すぐに反映されます。従来のプライバシーファイルは読み込まれません。更新前に内容を設定にコピーしてください。外部の法的情報 URL も廃止されます。既存ファイルは保持されます。

公開ページは `/privacy-policy`、`/terms-of-service`、`/legal-notice` です。認証情報が期限切れでも閲覧できます。`GET /api/legal` は現在のリビジョンと `cookie_banner` を返し、`GET /api/legal/:document` はサイズ制限付きのプレーンテキストを返します。`GET /api/privacy-policy` は別名として残ります。

`GET /api/settings/legal` と `PUT /api/settings/legal` は設定管理権限が必要です。JSON フィールドは `privacy_policy`、`terms_of_service`、`legal_notice`、`cookie_banner` です。

## ログインと登録

ログインと登録では、現在のポリシーと規約への明示的な同意が必要です。パスワード、Passkey、外部ログイン、二要素認証の完了にも適用されます。パッケージクライアントのパスワード認証は従来のプロトコル規則に従います。

クライアントは `GET /api/legal` で取得したリビジョンを `X-Renop-Legal-Revision` または同意 Cookie で受諾します。同意がない場合や古い場合は HTTP 428 と `X-Renop-Error-Code: legal_consent_required` を返します。成功時は既存の監査イベントに同意したリビジョンを記録します。

## Cookie の選択

浮動通知では必要なもののみ、すべて許可、カテゴリ別設定を選択できます。必須 Cookie はセッションとセキュリティに使用し、任意の外部検証には明示的な選択が必要です。通知を無効にしてもフッターから設定を再表示できます。

選択はブラウザーに一年間保存され、ポリシーまたは規約の変更で無効になります。任意サービスの拒否も有効な選択であり、許可されるまで連携サービスを読み込んではいけません。
