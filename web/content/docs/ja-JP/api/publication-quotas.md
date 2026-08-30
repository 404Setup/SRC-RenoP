---
title: 公開クォータ
order: 18
category: API リファレンス
description: アカウントとグローバルチームの期間別公開上限
---

# 公開クォータ

公開クォータは、ローカルでアップロードするファイル数、合計サイズ、完了したプロジェクト公開数を制限します。
新規環境の既定値は月ごとに 600 ファイル、40 MiB、20 回です。システム管理者は全体の既定値、または
アカウントとグローバルチームごとの設定を変更できます。

## ポリシー

`period` は `day`、`week`、`month` のいずれかで、境界は UTC です。ゼロは所有者別設定でのみ使用でき、
該当する操作を禁止します。管理者専用の `unlimited` は、その所有者のクォータ消費を無効にします。空の
設定オブジェクトを送信すると、すべての全体既定値に戻ります。

## 所有権

個人所有パッケージは公開アカウントのクォータを消費します。グローバルチームに関連付けたパッケージまたは
Maven 公開ドメインは、そのチームのクォータだけを消費します。所有権の移転は以後の公開にのみ適用され、
過去の使用量は移動しません。ミラーのダウンロードとカタログ更新はクォータ対象外です。

## 使用量の計上

Cargo と npm は承認されたバージョンごとに、保存ファイル一つと公開一回を計上します。Docker は manifest、
config、レイヤー記述子を数え、manifest 送信時に公開一回を計上します。Maven はクライアントの各 PUT を
ファイルとして数え、POM の受理時にプロジェクト公開一回を計上します。ファイルエンジンは各 PUT をファイル
一つ、公開一回として数えます。サーバー生成のインデックスとチェックサムは別途計上しません。

並行アップロードは、最初に有効期限付きの永続予約を作成します。検証成功時に使用量へ確定し、失敗または
放棄された予約は解放または定期削除されます。状態には確定済み使用量と有効な予約が含まれます。

## エンドポイント

```http
GET /api/publication-quota
GET /api/publication-quota/users/{username}
PUT /api/publication-quota/users/{username}
GET /api/publication-quota/super-teams/{prefix}
PUT /api/publication-quota/super-teams/{prefix}
GET /api/settings/publication-quota
PUT /api/settings/publication-quota
```

アカウントは自分の状態を、チームメンバーはチームの状態を参照できます。他アカウントの参照、個別設定、
`unlimited`、全体既定値の変更はシステム管理者だけが実行できます。

## 適用

上限到達時は `429 Too Many Requests` を返します。`X-Renop-Error-Code` は `publication_file_quota`、
`publication_byte_quota`、`publication_count_quota` のいずれかです。クォータは認証、権限、パッケージ予約、
名前空間関連付け、Maven ドメイン検証の後に適用され、権限そのものを付与しません。
