---
title: 状態とテレメトリ API
order: 9
category: API リファレンス
description: 公開ヘルス、実行時メトリクス、スナップショット、保護された診断
---

# 状態とテレメトリ API

明記されたレスポンスは protobuf です。ヘルスと現在状態は公開されます。メモリ診断には管理者権限と、
プロセス開始時から有効な `server.debug_mode` が必要です。

## 1. ヘルスとフロントエンドハッシュ

- **ヘルス**: `GET /api/status/health` はプロセスが応答中なら `"UP"` を返します。
- **ハッシュ**: `GET /api/status/hash` は再読込検出に使う埋め込みアセットハッシュを返します。

## 2. 現在のインスタンス状態

- **パス**: `GET /api/status/instance`
- **形式**: protobuf `InstanceStatus`。
- **内容**: バージョン、稼働時間、RSS/VSS、ディスク、goroutine、CPU、失敗数、debug、更新状態です。

### デコード例

```json
{
  "version": "1.0.0",
  "uptime": 86400,
  "used_memory": 33554432,
  "vss_memory": 268435456,
  "renop_used_disk": 5242880000,
  "disk_used": 107374182400,
  "disk_total": 536870912000,
  "used_threads": 24,
  "logical_cores": 16,
  "failures_count": 0,
  "debug_mode": false
}
```

## 3. 履歴スナップショットと診断

- **スナップショット**: `GET /api/status/snapshots` は時刻、メモリ、goroutine、open file、VSS を含む
  `StatusSnapshotList` を返します。
- **Heap profile**: `GET /api/debug/memory/heap`（`?gc=0` で事前 GC を省略）。
- **Allocation profile**: `GET /api/debug/memory/allocs`。
- **Goroutine profile**: `GET /api/debug/memory/goroutine`。
- **Runtime breakdown**: `GET /api/debug/memory/runtime`（`?gc=1` で事前 GC）。

```json
{
  "snapshots": [
    {
      "timestamp": 1787731200000,
      "used_memory": 33554432,
      "used_threads": 24,
      "open_files": 18,
      "vss_memory": 268435456
    }
  ]
}
```

バイナリ pprof は `go tool pprof` または Speedscope で開けます。起動時に debug mode が無効だった場合、
管理者でも診断ルートは `403` を返します。
