---
title: トークンと GPG 署名
order: 2
category: セキュリティと権限
description: パーソナルアクセストークン (PAT)、アップロードトークン、GPG 検証
---

# トークンと GPG 署名

- **PAT**: ユーザー権限を継承する個人用トークン
- **Upload Token**: CI/CD 用に特定リポジトリへの書き込みのみを許可するトークン
- **GPG 署名検証**: `require_gpg_signature: true` で未署名アーティファクトを `.renop.tmp.gpg` で隔離検証
