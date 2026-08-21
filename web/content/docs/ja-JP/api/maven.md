---
title: Maven
order: 4
category: API
---

# Maven の閲覧とヘルパー

プレフィックス: `/api/maven`（バッジは `/api/badge` 配下）

これらのエンドポイントはインデックスとメタデータを読みます。実体の成果物バイトは `/{repo}/group/artifact/…` にあります —
[storage.md](./storage.md) を参照。

パスパラメータは Maven レイアウトを使います。例:

```text
com/example/demo
com/example/demo/1.0.0
```

読み取り権限が不足している場合、多くの場合 `404 Not found` になります。

## ディレクトリとファイルの詳細（Protobuf）

### `GET /api/maven/details`

現在のユーザーに見えるリポジトリを仮想ルートとして包みます。

レスポンス: `FileDetails`（`application/x-protobuf`）

```text
type = DIRECTORY
name = "repositories"
files[] = { type: DIRECTORY, name: "<repo>" }
```

### `GET /api/maven/details/:repo_name`

リポジトリルート（子を含む）。

### `GET /api/maven/details/:repo_name/*`

パスの詳細。ディレクトリは `files` を、ファイルは `content_length` と `last_modified_time`（RFC3339Nano）を含みます。

`type` は `FILE` または `DIRECTORY`。

ファイルの `signed` は、そのパスの分離 GPG 署名が検証済みの場合だけ `true` になります。保護対象の Maven ファイルは `.jar`、
`.pom`、`.module` です。鍵の登録、アップロード、署名詳細の確認については
[GPG 署名](./gpg.md)を参照してください。

### `GET /api/maven/repo-details/:repo_name`

統計とミラー概要。レスポンス: `RepoDetailsResponse`。

| フィールド                                          | 意味                                               |
|-----------------------------------------------------|----------------------------------------------------|
| `name` / `visibility`                               | 名前、可視性                                       |
| `total_size` / `artifact_size` / `metadata_size`    | バイト数                                           |
| `total_files` / `artifact_count` / `metadata_count` | 件数（チェックサムと maven-metadata はメタデータ） |
| `mirrors[]`                                         | name, url, persist, cache_ttl, negative_cache, …   |

読み取り不可 → **403**（details は多くの場合 404 を使う点が異なる）。

## バージョン照会（Protobuf）

パスは `maven-metadata.xml` がある座標ディレクトリ（groupId/artifactId）を指す必要があります。

### `GET /api/maven/versions/:repo_name/*`

| クエリ   | デフォルト | 意味                         |
|----------|------------|------------------------------|
| `filter` | —          | バージョン部分文字列フィルタ |
| `sorted` | `true`     | 結果をソート                 |

レスポンス: `application/x-protobuf`、`VersionsResponse`

```protobuf
syntax = "proto3";

message VersionsResponse {
  bool is_snapshot = 1;
  repeated string versions = 2;
}
```

### `GET /api/maven/latest/version/:repo_name/*`

同じクエリパラメータ。`type=raw` で素のバージョン文字列（`text/plain`）。

デフォルトレスポンス: `application/x-protobuf`、`LatestVersionResponse`

```protobuf
syntax = "proto3";

message LatestVersionResponse {
  bool is_snapshot = 1;
  string version = 2;
}
```

### `GET /api/maven/latest/details/:repo_name/*`

最新一致成果物の `FileDetails`（`application/x-protobuf`）。

| クエリ       | デフォルト | 意味               |
|--------------|------------|--------------------|
| `extension`  | `jar`      | 拡張子             |
| `classifier` | —          | クラシファイア     |
| `filter`     | —          | バージョンフィルタ |

### `GET /api/maven/latest/file/:repo_name/*`

最新バージョンを解決し、ストレージ層経由で取得（リダイレクトまたは本文 — 直接成果物 URL に近い）。

## バッジ

### `GET /api/badge/latest/:repo_name/*`

最新バージョンの SVG バッジ。`Content-Type: image/svg+xml`。

| クエリ   | 意味                           |
|----------|--------------------------------|
| `name`   | 左ラベル（既定: リポジトリ名） |
| `color`  | 右の色（英数字または `#hex`）  |
| `prefix` | バージョン接頭辞               |
| `filter` | バージョンフィルタ             |

```markdown
![latest](https://your-host/api/badge/latest/releases/com/example/demo)
```

## POM 生成

### `POST /api/maven/generate/pom/:repo_name/*`

リポジトリへの書き込み権限が必要です。ボディ: `application/x-protobuf`、`PomDetails`（JSON も受付可）。

```protobuf
syntax = "proto3";

message PomDetails {
  string group_id = 1;
  string artifact_id = 2;
  string version = 3;
}
```

パスは既に `.pom` で終わるか、座標ディレクトリ（その場合ファイル名は `artifact_id-version.pom`）です。

ディスク不足 → 507。成功時は POM が書き込まれインデックスが更新されます。

## プライバシーポリシー

### `GET|HEAD /api/privacy-policy`

作業ディレクトリに `privacy-policy.txt` があれば `text/plain` で返します。なければ 404。Maven とは無関係で、 同じ API
グループにマウントされています。
