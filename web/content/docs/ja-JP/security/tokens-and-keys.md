---
title: API トークンと GPG 署名
order: 2
category: セキュリティと権限
description: きめ細かな API トークンと GPG 検証
---

# API トークンと GPG 署名

- **API トークン**: 名前、権限範囲、任意の有効期限を持つ 256 ビットの資格情報です。秘密値は作成時に
  一度だけ表示され、データベースには SHA-256 ダイジェストのみ保存されます。
- **認可**: 各操作ではトークンの範囲と所有アカウントの現在の権限を両方確認します。失効は即時反映されます。
- **送信方法**: API では `Authorization: Bearer <token>` を使用します。Basic はパッケージプロトコル専用で、
  Session ヘッダーと URL 内の資格情報は拒否されます。
- **GPG 署名検証**: `require_gpg_signature: true` で未署名アーティファクトを `.renop.tmp.gpg` で隔離検証
