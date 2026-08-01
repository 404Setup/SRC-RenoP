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
| `used_memory` / `total_memory`                         | 物理メモリ使用量および合計（バイト）               |
| `vss_memory`                                           | 仮想メモリサイズ（バイト）                         |
| `renop_used_disk`                                      | RenoP ストレージ使用量                             |
| `disk_used` / `disk_total`                             | ディスク使用量および合計                           |
| `used_threads` / `available_threads` / `total_threads` | ゴルーチンおよび同時実行スレッド関連               |
| `architecture` / `os`                                  | GOARCH / GOOS                                      |
| `logical_cores` / `physical_cores`                     | 論理および物理 CPU コア数                          |
| `failures_count`                                       | ランタイム失敗カウンタ                             |
| `update_state`                                         | アップデータ状態 — [updater.md](./updater.md) 参照 |
| `debug_mode`                                           | 起動時にデバッグモードが有効化されたか             |

## `GET /api/status/snapshots`

履歴サンプル。レスポンス: protobuf `StatusSnapshotList`。

| フィールド     | 意味               |
|----------------|--------------------|
| `timestamp`    | Unix ミリ秒        |
| `used_memory`  | メモリ             |
| `used_threads` | スレッド数         |
| `open_files`   | オープンファイル数 |

データがない場合は空リスト（404 ではない）。

## デバッグ分析 API（`/api/debug`）

**manager** 権限が必要であり、起動時に設定ファイルで `server.debug_mode: true`
が有効になっている必要があります。デバッグモードが無効な場合や権限が不足している場合は 403 を返します。

### `GET /api/debug/memory/heap`

Go ランタイムのヒーププロファイルをエクスポートします（pprof 形式）。

### `GET /api/debug/memory/allocs`

メモリ割り当てプロファイルをエクスポートします（pprof 形式）。

### `GET /api/debug/memory/goroutine`

Goroutine スタックプロファイルをエクスポートします（pprof 形式）。

### `GET /api/debug/memory/runtime`

Go ランタイムメモリ詳細（スタック / オフヒープ / RSS）を返します。レスポンス: `application/x-protobuf`、
`RuntimeMemoryBreakdown`。
