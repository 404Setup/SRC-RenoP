---
title: ストレージ
order: 8
category: API
---

# リポジトリ ストレージ パス

成果物は `/api` 配下ではありません。レイアウト:

```text
/{repo_name}/{maven-path}
```

デフォルト リポジトリ:

```text
/releases/...
/snapshots/...
/private/...
```

リポジトリ名は静的ルートと衝突してはいけません: `api`、`js`、`css`、`svg`、`assets`、`javadocs` など。

## メソッド

| メソッド   | 権限     | 動作                                                                      |
|------------|----------|---------------------------------------------------------------------------|
| GET        | 読み取り | ダウンロード。ブラウザの HTML 要求は管理 SPA にフォールバックする場合あり |
| HEAD       | 読み取り | ヘッダーのみ                                                              |
| PUT / POST | 書き込み | アップロード / 上書き                                                     |
| DELETE     | 書き込み | 削除。成功 204                                                            |

本文上限は約 2 GiB（`BodyLimit`）。アップロードはストリームされます。

### アップロード

```bash
curl -u admin:SECRET -T artifact.jar \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

典型的な成功: `201 Created`。再配置が無効でファイルが既にある場合、サーバーは上書きを拒否します（ 非 2xx は失敗として扱ってください）。

任意ヘッダー: `X-Generate-Checksums: true` は `.md5` / `.sha1` / `.sha256` / `.sha512` サイドカーを書き込みます。

サーバーはインデックス、任意のチェックサム、S3 同期を維持します。Maven クライアントからは通常のリポジトリレイアウトに見えます。

### マルチパート（分割）アップロード — 任意

上記の単一リクエスト `PUT` はそのままです。大きなファイルでは Web UI が並行分割アップロードを使う場合があります
（パート単位の再試行付き）。マシンクライアントも同じ API を使えます。

**マルチパートを使うとき:** ブラウザ UI は **8 MiB** 未満のファイルでは分割しません（単一 `PUT` の方が速い）。マシン
クライアントは任意サイズで分割セッションを開けます。非常に小さいペイロードはサーバーが 1 パートにまとめます。

プレフィックス: `/api/upload/chunked`（セッション Cookie / Basic / Bearer。対象リポジトリへの書き込み権限が必要）。

init と complete は **`application/x-protobuf`**（`ChunkedUploadInitRequest` /
`ChunkedUploadInitResponse` / `ChunkedUploadCompleteResponse`、`proto/api/v1/api.proto`）。パート本文は生バイナリ。

1. **`POST /api/upload/chunked/`** — セッション開始（`ChunkedUploadInitRequest` → `ChunkedUploadInitResponse`）

論理フィールド（snake_case）:

| フィールド           | 意味                                       |
|----------------------|--------------------------------------------|
| `purpose`            | `storage`（既定）                          |
| `path`               | 宛先 `repo/…/file`                         |
| `filename`           | 任意の表示名                               |
| `size`               | 総バイト数                                 |
| `generate_checksums` | チェックサム サイドカーを書く              |
| `chunk_size`         | 希望パートサイズ（任意。サーバーが正規化） |

レスポンス フィールド: `upload_id`、`chunk_size`、`chunk_count`、`purpose`。以降の `PUT` では返された
`chunk_size` / `chunk_count` を必ず使ってください。

**パートサイズ規則**（サーバー、`upload.NormalizeChunkSize`）:

| 総サイズ  | 典型的なパートサイズ        |
|-----------|-----------------------------|
| ≤ 256 KiB | 単一パート = ファイルサイズ |
| ≤ 8 MiB   | 単一パート = ファイルサイズ |
| ≤ 32 MiB  | 4 MiB                       |
| ≤ 128 MiB | 8 MiB                       |
| ≤ 512 MiB | 16 MiB                      |
| ≤ 2 GiB   | 24 MiB                      |
| それ以上  | 32 MiB（最大）              |

クライアントの `chunk_size` は **256 KiB … 32 MiB** にクランプされます。約 2048 パートを超える場合、サーバーは
パートサイズを上げます。`chunk_size` を省略（または `0`）すると上表を受け入れます。

2. **`PUT /api/upload/chunked/:upload_id/:index`** — 生パート本文（0 始まり）、並列可  
   成功: `204`。受理済み index の再 PUT は冪等（再試行安全）。

3. **`POST /api/upload/chunked/:upload_id/complete`** — 組み立て、インデックス、任意チェックサム  
   成功: `201` + `ChunkedUploadCompleteResponse`（`status=created`、`path=…`）。

4. **`DELETE /api/upload/chunked/:upload_id`** — 中止して一時データを破棄（`204`）。

完了しないセッションは約 **15 分** で期限切れ（一時ファイル削除）。失敗したパートはバックオフ付きで再試行してください。

### ダウンロード

```bash
curl -O "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

PUBLIC は認証不要。PRIVATE は Basic / Bearer。

ミラー設定時、ローカルに無いファイルは上流から取得される場合があります（リポジトリ設定ごとのキャッシュ / ネガティブキャッシュ）。

### 削除

```bash
curl -X DELETE -u admin:SECRET \
  "http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar"
```

## ブラウザアクセス

`Accept: text/html` のとき、欠けたリポジトリや一部ディレクトリは管理 SPA にフォールスルーし、
`http://host/releases/...` が UI を開けるようにします。マシンクライアントは `Accept: */*` を使うか Accept を省略して HTML
を避けてください。

## Javadoc プレビュー

有効時:

```text
GET /javadoc/:repo_name/*path-to-javadoc.jar
GET /javadoc/:repo_name/*path-to-javadoc.jar/raw/...
```

対応する読み取り権限が必要。`raw` は jar 内のファイルを配信。サイズは `max_javadoc_size_mb` で制限。

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

`~/.m2/settings.xml` でその server id にユーザー名 + パスワード（またはアップロードトークン）を設定します。
