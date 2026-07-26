---
title: アップデータ
order: 7
category: API
---

# アップデータ

プレフィックス: `/api/updater`

`GET /status` は公開。`check` / `install` / `upload` / `restart` には **manager** が必要です。

同じ状態は `GET /api/status/instance` の `update_state` にもあります。

典型的な流れ:

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

レスポンス: `application/x-protobuf`、`UpdateState`（`proto/api/v1/api.proto` 参照）。

| フィールド             | 意味                                                            |
|------------------------|-----------------------------------------------------------------|
| `status`               | `idle`、`available`、`downloading`、`ready_to_restart`、`error` |
| `latest_version`       | 最新バージョン文字列                                            |
| `download_url`         | パッケージのダウンロード URL                                    |
| `progress`             | ダウンロード中は 0–100                                          |
| `error_message`        | `status` が `error` のとき設定                                  |
| `size`                 | パッケージサイズ（バイト）                                      |
| `estimated_disk_space` | 必要な空き容量の見積もり（バイト）                              |
| `release_date`         | リリース日時文字列                                              |
| `release_notes`        | リリースノート本文                                              |
| `commit_sha`           | ソースコミット                                                  |
| `is_release`           | リリースチャネルのビルド                                        |

## `POST /api/updater/check`

| クエリ    | デフォルト | 意味                       |
|-----------|------------|----------------------------|
| `channel` | `release`  | `release` または `nightly` |

```json
{
  "has_update": true,
  "current_version": "…",
  "latest_version": "…",
  "download_url": "…",
  "channel": "release",
  "size": 12345678,
  "estimated_disk_space": 40000000,
  "release_date": "…",
  "release_notes": "…",
  "commit_sha": "…",
  "is_release": true
}
```

チェック失敗 → 500、`{ "error": "…" }`。

## `POST /api/updater/install`

現在の `download_url` を使って非同期でダウンロードと展開。空の場合は nightly の既定 URL にフォールバック。

| ステータス | 理由                                                           |
|------------|----------------------------------------------------------------|
| 507        | ディスク不足                                                   |
| 409        | インストールが既に実行中（`Installation already in progress`） |

即時の成功レスポンス:

```json
{"status": "started"}
```

進捗は `/status` をポーリング。完了状態: `ready_to_restart`。

## `POST /api/updater/upload`

オフライン更新: multipart zip。フォームフィールド `file` または `package`。`.zip` 必須。

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

この単一リクエストの multipart 経路は、小さなパッケージと非 UI クライアントの既定のままです。

### マルチパート オフライン アップロード — 任意

Dashboard のオフライン更新ダイアログからの大きな zip は、共有セッション API による並行分割アップロードを使う場合があります
（manager のみ）。 **8 MiB** 未満のパッケージは引き続き単一リクエストの
`POST /api/updater/upload` を使います。init/complete は **`application/x-protobuf`**
（`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`）。パートは生のオクテットです。

パートサイズは総サイズから動的に決まります（[storage.md](./storage.md) のマルチパート節参照）。 init レスポンスの
`chunk_size` / `chunk_count` を使ってください。

1. `POST /api/upload/chunked/` に `purpose=updater`、`filename`（`.zip` で終わること）、`size`
2. 各パートを並列 `PUT /api/upload/chunked/:id/:index`（再試行安全。受理済みパートの再 PUT 可）
3. `POST /api/upload/chunked/:id/complete` — バイナリを展開し `ready_to_restart` に設定

complete の protobuf フィールド: `status=ready_to_restart`、`message=…`。

## `POST /api/updater/restart`

準備済み更新でバイナリを置換して再起動。

準備ができていない → 400（`No update ready to install`）。

```json
{"status": "restarting"}
```

その後接続は切断されます。想定どおりの動作です。
