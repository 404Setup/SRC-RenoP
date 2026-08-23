---
title: Maven メタデータ API
order: 4
category: API リファレンス
description: アーティファクト検索、詳細、バージョン、バッジ生成
---

# Maven メタデータ API

- `GET /api/search?q=...` - アーティファクト検索
- `GET /api/maven/details/:repo/:group/:artifact` - 詳細情報
- `GET /api/maven/badge/:repo/:group/:artifact/version.svg` - バッジ生成
