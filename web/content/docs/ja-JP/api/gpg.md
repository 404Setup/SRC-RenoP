---
title: GPG 署名
order: 5
category: API
description: 署名鍵の登録と Maven 成果物の署名検証
---

# GPG 署名

RenoP は Maven 成果物の分離 OpenPGP 署名を検証します。GPG ポリシーの対象は `.jar`、`.pom`、`.module`
ファイルです。アップロードを行うアカウントに登録された鍵で検証が完了した場合に限り、署名レコードが 保存されます。

## 設定

`config.yaml` の `server.gpg.key_servers` に、1 個から 8 個までの HTTPS 鍵サーバーを設定します。同じ設定は 設定 API の
`server.gpg` フィールドからも変更できます。鍵の登録時、RenoP はこれらのサーバーを使用して鍵 ID
またはフィンガープリントを解決します。[設定の概要](../configuration/overview.md) と[設定 API](./settings.md)
も参照してください。

リポジトリで `require_gpg_signature: true` を指定すると、上記 3 種類の保護対象ファイルに署名が必須に なります。チェックサムファイルと
Maven メタデータの付属ファイルも同じ公開処理で扱われます。詳しくは
[リポジトリとミラー](../configuration/repositories.md)を参照してください。

## 鍵の登録

認証済みユーザーは、プロフィールに最大 10 個の公開鍵を登録できます。

| メソッド | エンドポイント                       | 結果                      |
|----------|--------------------------------------|---------------------------|
| `GET`    | `/api/auth/profile/gpg`              | `GpgKeyList`              |
| `POST`   | `/api/auth/profile/gpg`              | `GpgKeyDto`               |
| `DELETE` | `/api/auth/profile/gpg/:fingerprint` | 空の `204` レスポンス     |
| `GET`    | `/api/auth/profile/gpg/releases`     | 公開履歴 `GpgReleaseList` |

`POST` の本文は `GpgKeyReferenceRequest`（`application/x-protobuf`）です。

```protobuf
syntax = "proto3";

message GpgKeyReferenceRequest {
  string key_id = 1;
}
```

短い鍵 ID が複数の公開鍵に一致する場合は、完全なフィンガープリントを指定してください。サーバーは解決した
公開鍵をデータベースに保存し、秘密鍵のデータは受け付けません。これらのエンドポイントには認証が必要で、
公開履歴を参照できるのは該当ユーザーだけです。

## 署名付き成果物のアップロード

成果物と分離署名は同じ Maven パスにアップロードします。署名ファイル名は成果物のファイル名に小文字の
`.asc` を付けたものにしてください（例: `demo-1.0.0.jar.asc`）。

1 回のリクエストで成果物と署名を同じバッチに含める場合は、成果物のリクエストに
`X-RenoP-GPG-Signature-Expected: true` を指定します。

```bash
curl -u alice:TOKEN \
  -H 'X-RenoP-GPG-Signature-Expected: true' \
  -T demo-1.0.0.jar \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar'

curl -u alice:TOKEN \
  -T demo-1.0.0.jar.asc \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar.asc'
```

分割アップロードでは、HTTP ヘッダーの代わりに `ChunkedUploadInitRequest` の
`gpg_signature_expected: true` を設定します。ブラウザーのアップロードフォームは、一致する `.asc` ファイル
を検出するとこの値を自動的に設定します。

署名は 1 MiB 以下の ASCII Armor 形式の OpenPGP 署名でなければなりません。署名鍵はアップロードユーザーが
登録した鍵のいずれかである必要があります。リポジトリが署名を必須としている場合、またはアップロードで
明示的に署名を要求した場合、検証が完了するまで成果物は GPG 隔離領域に保持されます。一致するファイルが 揃わない公開処理は約
15 分後に期限切れとなり、失敗として記録されます。

署名が任意で、待機フラグを付けずに成果物をアップロードした場合は、未署名ファイルとして公開されます。 後から `.asc`
をアップロードして検証済み署名レコードを作成することもできます。成果物を置き換えると、 新しい公開処理が検証されるまで以前の署名レコードは無効になります。

## 検証結果の確認

### `GET /api/maven/signatures/:repo_name/*`

検証済みの `.jar`、`.pom`、`.module` 成果物について `GpgSignatureDetails`（`application/x-protobuf`）を
返します。リポジトリの読み取り権限が必要です。レコードが存在しない場合、対象外の拡張子の場合、または 成果物にアクセスできない場合は
`404` を返します。

| フィールド                                | 内容                                |
|-------------------------------------------|-------------------------------------|
| `repository` / `artifact_path`            | リポジトリ名と Maven 相対成果物パス |
| `fingerprint` / `key_id`                  | 署名に使用した公開鍵の識別子        |
| `primary_identity`                        | 解決された公開鍵の主 ID             |
| `uploader`                                | 公開処理を送信したアカウント        |
| `signature_created_at` / `verified_at`    | Unix 時刻（ミリ秒）                 |
| `hash_algorithm` / `public_key_algorithm` | 検証時に記録されたアルゴリズム      |

`FileDetails.signed` は、そのファイルに検証済み署名レコードがある場合だけ `true` になります。ブラウザーは
署名済み成果物にロック操作ボタンを表示し、クリックすると署名詳細ダイアログを開いて上記エンドポイントを
読み込みます。対応しているテキストファイルのプレビューは引き続き利用でき、`.pom` と `.module` も対象です。

公開処理の失敗と理由は `GET /api/auth/profile/gpg/releases` で確認できます。状態は `queued`、`validating`、
`success`、`failed` です。
