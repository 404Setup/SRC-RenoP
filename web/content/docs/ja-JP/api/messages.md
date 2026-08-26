---
title: メッセージセンター API
order: 7
category: API リファレンス
description: アカウント通知、未読数、ワークフロー操作、管理者アナウンス
---

# メッセージセンター API

すべてのルートで認証が必要です。レスポンスは既定で protobuf を使用し、キャッシュされません。API Token は
`messages:read` が必要で、管理者送信には `admin:notifications` と管理者ロールも必要です。

## メッセージの一覧と消去

- **一覧**: `GET /api/messages?limit=30&cursor=...`
- **解決済みメッセージの消去**: `DELETE /api/messages`
- `limit` は 1～100 です。`cursor` は前ページの不透明な `next_cursor` です。
- ワークフロー操作が `pending` のメッセージは一括消去されません。

### デコード後のレスポンス例

```json
{
  "messages": [
    {
      "id": "00000000-0000-4000-8000-000000000001",
      "kind": "announcement",
      "severity": "info",
      "title": "Maintenance",
      "body": "Maintenance starts at 02:00 UTC.",
      "action_status": "",
      "created_at": 1787731200000,
      "read_at": 0
    }
  ],
  "unread_count": 1,
  "next_cursor": ""
}
```

## 未読数の取得

- **パス**: `GET /api/messages/unread-count`
- **デコード後のレスポンス**: `{"unread_count":3}`

## 既読化と削除

### 1 件

- **既読化**: `POST /api/messages/:id/read`
- **削除**: `DELETE /api/messages/:id`
- 他アカウントのメッセージは `404`、未解決のワークフローは `409` になります。

### 全件

- **すべて既読化**: `POST /api/messages/read-all`
- レスポンスには更新件数が含まれます。

## 管理者アナウンスの送信

- **宛先検索**: `GET /api/messages/admin/users?q=alice` は最大 8 件を返します。
- **送信**: `POST /api/messages/admin`
- 全アカウントには `all: true`、個別送信には正確な `recipients` を指定します。タイトル、本文、重要度、
  宛先数にはサーバー側の上限があります。

```json
{
  "recipients": ["alice", "bob"],
  "all": false,
  "severity": "warning",
  "title": "Scheduled maintenance",
  "body": "The service will restart at 02:00 UTC."
}
```

招待とシステム結果は担当サービスが作成します。チームからの削除通知はリポジトリとパッケージ、または
Maven ドメインを示しますが、操作したメンバーは意図的に表示しません。
