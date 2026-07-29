---
title: アップデータ
order: 7
category: API
---

# アップデータ

プレフィックス: `/api/updater`

`GET /status` は公開。`check` / `install` / `upload` / `restart` には **manager** が必要。

状態は `GET /api/status/instance` の `update_state` にもある。

```text
idle → available → downloading → ready_to_restart
              ↘ error
```

## `GET /api/updater/status`

レスポンス: `application/x-protobuf`、`UpdateState`（`proto/api/v1/api.proto`）。

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

| クエリ    | デフォルト               | 意味                       |
|-----------|--------------------------|----------------------------|
| `channel` | 設定 `updater.channel`   | `release` または `nightly` |

省略 / 不正 → `updater.channel`（既定 `release`）。

| チャネル    | `info.json`                                           |
|-------------|-------------------------------------------------------|
| `nightly`   | `https://mvnc.pkg.one/update/renop/nightly/info.json` |
| `release`   | `https://mvnc.pkg.one/update/renop/stable/info.json`  |

パッケージ: `…/{nightly\|stable}/{version}/{file}`。

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

失敗 → 500、`{ "error": "…" }`。

## `POST /api/updater/install`

現在の `download_url` で非同期ダウンロード / 展開。

| ステータス | 理由                                                           |
|------------|----------------------------------------------------------------|
| 507        | ディスク不足                                                   |
| 409        | インストール実行中（`Installation already in progress`）       |

```json
{"status": "started"}
```

`/status` をポーリング。完了: `ready_to_restart`。

## `POST /api/updater/upload`

オフライン更新: multipart zip（`file` または `package`）。`.zip` 必須。

```json
{
  "status": "ready_to_restart",
  "message": "Offline update installed successfully"
}
```

### マルチパートアップロード（任意）

大きな zip はチャンクアップロード可（manager）。**8 MiB** 未満 → 単一 `POST /api/updater/upload`。

init/complete: **`application/x-protobuf`**（`ChunkedUploadInitRequest` / `ChunkedUploadCompleteResponse`）。パートは生バイト。

パートサイズは [storage.md](./storage.md) 参照。init の `chunk_size` / `chunk_count` を使う。

1. `POST /api/upload/chunked/` — `purpose=updater`、`filename`（`.zip`）、`size`
2. `PUT /api/upload/chunked/:id/:index`（並列・再試行可）
3. `POST /api/upload/chunked/:id/complete` → `ready_to_restart`

## `POST /api/updater/restart`

準備済みの更新バイナリがある場合は適用してプロセスを再起動します。ない場合は更新を適用せず、現行プロセスのみ再起動します。

```json
{"status": "restarting"}
```
