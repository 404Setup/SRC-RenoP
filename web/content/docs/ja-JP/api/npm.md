---
title: npm レジストリ API
order: 7
category: API リファレンス
description: npm メタデータ、公開、tarball、dist-tag、チーム、管理エンドポイント
---

# npm レジストリ API

`npm` 形式の各リポジトリは `/{repo}/` 配下に npm 互換 JSON レジストリを公開します。初回公開前に管理 API
または Web UI からパッケージ名を予約してください。

## レジストリ検出と認証ユーザー

- **可用性**: `GET /{repo}/-/ping`
- **現在のアカウント**: `GET /{repo}/-/whoami`
- **検索**: `GET /{repo}/-/v1/search?text={query}&size={limit}&from={offset}`

プロトコルエラーは安定した `error` と `reason` を持つ JSON です。

```json
{
  "error": "not_found",
  "reason": "npm package was not found"
}
```

## パッケージメタデータと tarball

- **完全または省略 packument**: `GET /{repo}/{package}`
- **Tarball**: `GET /{repo}/{package}/-/{name}-{version}.tgz`
- **公開またはメタデータ編集**: `PUT /{repo}/{package}`

scoped 名は `%40example%2Flibrary` のように単一 path parameter へ encode できます。packument は ETag と
Last-Modified に対応します。`application/vnd.npm.install-v1+json` にはサイズ制限付き省略メタデータを返し、
非公開レスポンスは共有キャッシュを無効化します。

公開 document は SemVer 1 件と base64 tarball 1 件を含められます。JSON は 96 MiB、圧縮 tarball は 64 MiB、
展開後は 512 MiB、file entry は 100,000、`package.json` は 2 MiB が上限です。package ごとに最大 5,000
version と合計 4 MiB の version metadata を保持します。server は decode 結果を staging へ stream し、
検証途中の tarball を公開しません。

## Dist-tag とライフサイクル

- **タグ一覧**: `GET /{repo}/-/package/{package}/dist-tags`
- **タグ設定**: `PUT /{repo}/-/package/{package}/dist-tags/{tag}`
- **タグ削除**: `DELETE /{repo}/-/package/{package}/dist-tags/{tag}`
- **revision 付き更新または公開取消**: `PUT /{repo}/{package}/-rev/{revision}`
- **revision 付きパッケージ削除**: `DELETE /{repo}/{package}/-rev/{revision}`

バージョンは不変です。公開取消と削除は tombstone を作るため、公開済み番号は再利用できません。revision の
競合は `409 Conflict` となり、クライアントは現在の packument を再取得します。

## ブラウザ管理 API

same-origin 管理エンドポイントは JSON を使い、失敗時は安定した `X-Renop-Error-Code` header を返します。

- `GET /api/npm/repositories/{repo}/packages`
- `POST /api/npm/repositories/{repo}/packages`
- `PUT /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/versions?package={package}&version={version}`
- `GET /api/npm/repositories/{repo}/owners?package={package}`
- `POST /api/npm/repositories/{repo}/owners?package={package}`
- `PUT /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `DELETE /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `GET /api/npm/repositories/{repo}/users/search?package={package}&q={query}`
- `POST /api/npm/repositories/{repo}/invitations/{id}/{accept|reject}`

catalog は 1 から 100 の `limit` と bounded `offset` でページングします。非公開パッケージはメンバーまたは
管理者以外に表示されません。チーム詳細は L3/L4 メンバーと管理者だけに返します。

## 認証と認可

npm client はパスワードまたは API Token の Basic、もしくは API Token の `_authToken` を利用できます。
Bearer scope はアカウントの現在の権限と正確なリポジトリ、パッケージ、チーム対象で制限されます。公開には既存
パッケージと L1、メタデータ・公開取消には L2、チーム変更には L3、所有権・パッケージ削除には L4 が必要です。
