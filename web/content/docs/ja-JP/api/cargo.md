---
title: Cargo レジストリ API
order: 5
category: API リファレンス
description: Sparse Index、crate の公開、ダウンロード、yank
---

# Cargo レジストリ API

RenoP は Cargo Registry と Sparse Index の仕様を実装します。

## Sparse Index 設定 (`config.json`)

- **パス**: `GET /{repo}/config.json` または `GET /{repo}/index/config.json`
- **用途**: Cargo が初回接続時に読み、レジストリの API を検出します。

### JSON レスポンス

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## Sparse Index メタデータ

- **パス**: `GET /{repo}/index/{prefix}/{crate_name}`
- **用途**: Cargo 標準の crate 名シャーディングに従った行区切り JSON を返します。

---

## crate の公開

- **パス**: `PUT /{repo}/api/v1/crates/new`
- **認証**: `Authorization: <token>` の Token が必要です。
- **本文**: 4 バイトの JSON 長、JSON メタデータ、`.crate` バイナリアーカイブの順です。
- **名前競合**: 正規化名がローカルまたは適用対象ミラーに存在する最初の公開は `409 Conflict` になります。
  上流確認が確定できない場合は `503 Service Unavailable` を返します。

ローカル公開では、検証済み `Cargo.toml` の `package.readme` 宣言を読み、crate 全体をメモリーに保持せずに
対象ファイルをアーカイブから抽出します。パッケージ詳細は最大 512 KiB の Markdown を返し、ブラウザーは
共通の要素と URL の許可リストで描画します。カタログと検索は README 本文を読み込みません。

---

## crate のダウンロード

- **パス**: `GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **レスポンス**: `application/x-tar` の `.crate` アーカイブです。

---

## yank と unyank

- **Yank**: `DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank**: `PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **認証**: crate 所有者または管理者です。

## リソースのロック

管理者とこのリポジトリのモデレーターは、有効なブラウザーセッション Cookie を使用して
`PUT /{repo}/api/v1/crates/{crate_name}/locks` を呼び出します。API トークンやパッケージクライアントの認証情報では管理できません。

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

パッケージ全体をロックするには `version` を省略するか `""` にします。モードは `write` と `read` です。
公開理由は翻訳に対応した `hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、
`squatting`、`quality` です。成功時は `{"ok":true}` を返します。指定した手動ロックを解除するには、
`DELETE /{repo}/api/v1/crates/{crate_name}/locks` に `{"version":"1.2.3"}` を送信します。

書き込み禁止は変更と上流からの更新を停止しますが、保存済みファイルのダウンロードは維持します。
読み取り禁止は担当者を含む全員のファイルダウンロードとドキュメントプレビューも禁止します。
メタデータを閲覧できるのは管理者、対象リポジトリのモデレーター、所有者、共同作業者のみです。
L0 メンバーと紐付けられたグローバルチームのメンバーも含みます。他の閲覧者には、ロックされたパッケージや
バージョンをメタデータ、sparse インデックス、検索、プロフィール、チームのリソース一覧に表示しません。

パッケージとバージョンのメタデータは、公開 `locks` に `mode`、`reason`、`source`、`locked_at` を含みます。
システムロックと手動ロックは独立しており、手動ロックの解除ではシステム制限を解除しません。
変更の拒否は `423` と `X-Renop-Error-Code: resource_locked`、読み取りの拒否は `404` を返します。
バージョンがロックされている間はパッケージ全体のアーカイブ、永久的な廃止、削除は禁止されますが、他のバージョンは公開できます。
リソースのロックが存在するリポジトリの再設定と削除も禁止されます。
