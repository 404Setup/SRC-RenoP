---
title: ストレージ
order: 8
category: API
---

# リポジトリ ストレージ パス

成果物パスは `/api` 配下ではありません。レイアウト:

```text
/{repo_name}/{maven-path}
```

既定リポジトリ:

```text
/releases/...
/snapshots/...
/private/...
```

リポジトリ名は `api`、`js`、`css`、`svg`、`assets`、`javadoc` などの静的ルート接頭辞と衝突してはなりません。

## メソッド

| メソッド   | 権限 | 動作                                                                      |
|------------|------|---------------------------------------------------------------------------|
| GET        | 読   | ダウンロード。HTML Accept のブラウザ要求は管理 SPA にフォールバックし得る |
| HEAD       | 読   | 応答ヘッダのみ                                                            |
| PUT / POST | 書   | アップロードまたは上書き                                                  |
| DELETE     | 書   | 削除。成功時 `204`                                                        |

最大 body サイズは約 2 GiB（`BodyLimit`）。アップロードはストリーム処理されます。

### アップロード

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

成功時は `201 Created` を返します。redeploy が無効で対象アーティファクトが既に存在する場合、`409 Conflict` になります。Maven メタデータ（`maven-metadata.xml` とそのチェックサムまたは署名の付随ファイル）は引き続き更新できます。

任意のリクエスト ヘッダ `X-Generate-Checksums: true` は `.md5`、`.sha1`、`.sha256`、`.sha512` サイドカーを書き込みます。

サーバは設定に従い成果物インデックス、任意チェックサム、S3 同期を更新します。クライアントから見えるのは標準の Maven リポジトリ
レイアウトです。

### チャンク アップロード（任意）

認証はストレージ書き込みと同じです。セッション Cookie、Basic、または Bearer と、対象リポジトリへの書き込み権限が必要です。

プレフィックス: `/api/upload/chunked`

ブラウザ UI は **8 MiB** 以上のファイルでチャンク アップロードを使い、それ未満は単一 `PUT` を使います。非ブラウザ
クライアントは任意サイズでチャンク セッションを開けます。サーバは極小のペイロードを単一部分にまとめることがあります。

init と complete は **`application/x-protobuf`**（`proto/api/v1/api.proto` の `ChunkedUploadInitRequest`、
`ChunkedUploadInitResponse`、`ChunkedUploadCompleteResponse`）を使います。パート body は raw バイナリです。

1. **`POST /api/upload/chunked/`** — セッション作成（`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`）

| フィールド           | 説明                                      |
|----------------------|-------------------------------------------|
| `purpose`            | `storage`（既定）                         |
| `path`               | 宛先パス `repo/…/file`                    |
| `filename`           | 任意の表示名                              |
| `size`               | 総バイト数                                |
| `generate_checksums` | チェックサム サイドカーを書くか           |
| `chunk_size`         | 希望パート サイズ（任意。サーバが正規化） |

応答フィールド: `upload_id`、`chunk_size`、`chunk_count`、`purpose`。以降のパート アップロードは返却された `chunk_size` と
`chunk_count` を使用する必要があります。

**パート サイズ規則**（サーバ、`upload.NormalizeChunkSize`）:

| 総サイズ  | パート サイズ              |
|-----------|----------------------------|
| ≤ 256 KiB | 単一部分 = ファイル サイズ |
| ≤ 8 MiB   | 単一部分 = ファイル サイズ |
| ≤ 32 MiB  | 4 MiB                      |
| ≤ 128 MiB | 8 MiB                      |
| ≤ 512 MiB | 16 MiB                     |
| ≤ 2 GiB   | 24 MiB                     |
| それ以上  | 32 MiB（上限）             |

クライアント指定の `chunk_size` は **256 KiB … 32 MiB** にクランプされます。パート数が約 2048 を超える場合、サーバはパート
サイズを上げます。`chunk_size` を省略するか `0` を送ると上表を使います。

2. **`PUT /api/upload/chunked/:upload_id/:index`** — raw パート body（0 始まり）。並列アップロード可  
   成功: `204`。受理済み index の再アップロードは冪等です。

3. **`POST /api/upload/chunked/:upload_id/complete`** — 組み立て、インデックス更新、任意チェックサム  
   成功: `201` と `ChunkedUploadCompleteResponse`（`status=created`、`path=…`）。

4. **`DELETE /api/upload/chunked/:upload_id`** — セッション中止と一時データの破棄（`204`）。

未完了セッションは約 **15 分** で期限切れとなり、一時データは削除されます。

### ダウンロード

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC リポジトリは認証不要です。PRIVATE は Basic または Bearer が必要です。

ミラー設定時、ローカルに無いオブジェクトはリポジトリ単位の cache / negative-cache 設定に従い上流から取得されることがあります。

### 削除

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## ブラウザ アクセス

`Accept: text/html` のとき、欠落リポジトリや一部ディレクトリは管理 SPA にフォールバックし、`http://host/releases/...`
のようなパスで UI を開けます。機械クライアントは HTML を避けるため `Accept: */*` を送るか `Accept` を省略してください。

## Javadoc プレビュー

有効時:

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

対応する読み取り権限が必要です。`raw` 形式は jar 内ファイルを配信します。サイズは `max_javadoc_size_mb` で制限されます。

## Maven 設定例

```xml

<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>

<distributionManagement>
<repository>
    <id>renop</id>
    <url>http://localhost:3000/releases</url>
</repository>
<snapshotRepository>
    <id>renop</id>
    <url>http://localhost:3000/snapshots</url>
</snapshotRepository>
</distributionManagement>
```

`~/.m2/settings.xml` で、対応する server `id` に username と password（または upload token）を設定します。
