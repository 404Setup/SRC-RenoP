---
title: ステータス
order: 5
category: API
---

# ステータスとヘルス

プレフィックス: `/api/status`

認証は不要です。

## `GET /api/status/health`

```json
"UP"
```

Liveness プローブ。

## `GET /api/status/hash`

フロントエンド資産のコンテンツハッシュを JSON 文字列として返します（キャッシュ破棄用）。

## `GET /api/status/instance`

レスポンス: `application/x-protobuf`、`InstanceStatus`。

| フィールド                                             | 意味                                               |
|--------------------------------------------------------|----------------------------------------------------|
| `version`                                              | バイナリ版                                         |
| `development`                                          | 開発ビルドフラグ                                   |
| `uptime`                                               | 起動からのミリ秒                                   |
| `used_memory` / `total_memory`                         | メモリ（おおよそ MiB）                             |
| `renop_used_disk`                                      | RenoP ストレージ使用量                             |
| `disk_used` / `disk_total`                             | ディスク                                           |
| `used_threads` / `available_threads` / `total_threads` | スレッド / ゴルーチン関連                          |
| `architecture` / `os`                                  | GOARCH / GOOS                                      |
| `logical_cores` / `physical_cores`                     | CPU                                                |
| `failures_count`                                       | ランタイム失敗カウンタ                             |
| `update_state`                                         | アップデータ状態 — [updater.md](./updater.md) 参照 |

## `GET /api/status/snapshots`

履歴サンプル。レスポンス: protobuf `StatusSnapshotList`。

| フィールド     | 意味               |
|----------------|--------------------|
| `timestamp`    | Unix ミリ秒        |
| `used_memory`  | メモリ             |
| `used_threads` | スレッド数         |
| `open_files`   | オープンファイル数 |

データがない場合は空リスト（404 ではない）。
