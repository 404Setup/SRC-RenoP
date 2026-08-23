---
title: Cargo レジストリ API
order: 5
category: API リファレンス
description: スパースインデックス、Crate 公開・ダウンロード・yank
---

# Cargo レジストリ API

- `GET /{repo}/config.json` - レジストリ設定
- `GET /{repo}/index/...` - インデックスメタデータ
- `PUT /{repo}/api/v1/crates/new` - Crate 公開
- `GET /{repo}/api/v1/crates/:crate/:version/download` - ダウンロード
- `DELETE /{repo}/api/v1/crates/:crate/:version/yank` - ヤンク
