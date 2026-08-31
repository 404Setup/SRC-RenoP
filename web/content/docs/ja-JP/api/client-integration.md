---
title: HTTP API 連携
order: 19
category: API リファレンス
description: API の選択、protobuf media type、認証情報、エラー、再試行、クライアント互換性
---

# HTTP API 連携

RenoP は同じ origin から management endpoint と複数の package protocol を提供します。Media type や credential を選ぶ前に
API family を
特定してください。すべての route を JSON REST endpoint として扱うのは誤りです。

## 正しい API サーフェスを選ぶ

| サーフェス              | 主なパス                                    | 想定クライアント                      |
|:------------------------|:--------------------------------------------|:--------------------------------------|
| Management・browser API | `/api/...`                                  | RenoP UI、管理 tool、automation       |
| Maven・generic file     | `/{repo}/{path}`                            | Maven、Gradle、HTTP artifact client   |
| Cargo sparse registry   | `/{repo}/config.json`、`/{repo}/api/v1/...` | Cargo と互換 tool                     |
| npm registry            | `/{repo}/{package}`、`/{repo}/-/...`        | npm-compatible client                 |
| Docker/OCI Distribution | `/v2/...`、`/v2/token`                      | Docker、Podman、OCI client            |
| Documentation preview   | `/javadoc/...`、`/cargodoc/...`             | Repository authorization 後の browser |

Native package URL に `/api` を付けないでください。Package protocol の method/error shape から management semantics
を推測しないでください。

## 宣言された表現形式を使う

多くの management request/response は `application/x-protobuf` です。OpenAPI schema は logical field を説明しますが、example
が endpoint の
JSON compatibility を意味するわけではありません。同じ RenoP release の `proto/api/v1/api.proto` message を使用します。

Protobuf body の endpoint では両方を明示します。

```http
Content-Type: application/x-protobuf
Accept: application/x-protobuf
```

Health check と一部 error は plain text です。Cargo、npm、Docker/OCI は protocol が要求する structured JSON または binary
format を使います。
Path suffix から推測せず endpoint documentation に従ってください。

## 呼び出し元に合う認証情報を選ぶ

| 認証情報               | 用途                                               | 重要な制限                                  |
|:-----------------------|:---------------------------------------------------|:--------------------------------------------|
| `renop_session` cookie | Interactive browser UI と private account-security | HttpOnly。Script へ抽出・再利用しない       |
| Bearer API token       | Management automation と対応 route                 | Account/team の現在の permission と積を取る |
| HTTP Basic             | Package client と指定 upload flow                  | Session/Bearer の一般的な代替ではない       |
| Docker Bearer token    | Docker/OCI Distribution operation                  | Registry challenge と token exchange で取得 |

Token secret は作成時だけ表示されます。Secret manager に保存し、expiration、target、scope を最小化し、job/device 廃止時に
revoke します。
Query credential と `Authorization: Session` は拒否されます。

## ベース URL を正しく構成する

Production では一つの正規 HTTPS origin を使います。Cookie、redirect、Docker challenge、generated repository URL が public
service を指すよう、
reverse proxy は元の `Host` と scheme を保持します。

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

成功時の response body は `"UP"` です。

Health endpoint は reachability 用で、database/storage commit を証明しません。Deployment automation で dependency
を検証する場合は、別の
認証済み readiness operation を実行します。

## 安定した順序でレスポンスを処理する

1. HTTP status を読む。
2. Response `Content-Type` を確認する。
3. 存在する場合は `X-Renop-Error-Code` を読む。
4. 対応する protocol decoder だけで body を decode する。
5. Credential を除き、timestamp と sanitized context を記録する。

Management failure は短い plain text の場合があります。Docker Distribution、Cargo、npm は native structured error
を保持します。完全な英語文を
branch condition にしないでください。

## ステータスをクライアント動作へ割り当てる

| ステータス  | クライアント動作                                                                     |
|:------------|:-------------------------------------------------------------------------------------|
| `200`–`204` | 文書化された type で decode。仕様上の empty success body も有効                      |
| `202`       | Accepted だが visible とは限らない。Publication review が pending の場合がある       |
| `302`       | Authorized S3 presigned URL など、文書化された download だけ追従                     |
| `400`       | Request を修正。自動 retry は同じ failure を繰り返すことが多い                       |
| `401`       | Credential type が許可されるか確認してから refresh/replace                           |
| `403`       | Blind retry しない。Scope、target、permission、team、policy、debug mode の変更が必要 |
| `404`       | Path と visibility を確認。Private/hidden data は意図的に隠される場合がある          |
| `409`       | State を読み直し、immutable/concurrent operation を変更できるか判断                  |
| `413`       | 妥当な場合だけ payload を縮小し、それ以外は proxy/server limit を修正                |
| `429`       | Retry guidance、jitter、lower concurrency を使用                                     |
| `5xx`       | Bounded safe operation だけ retry。Original error を残し dependency を確認           |

## セマンティクスが許す場合だけ再試行する

Transport failure 後の GET/HEAD は一般に安全です。Write は idempotency と、切断前に server commit 済みの可能性を確認します。Jitter
を含む
bounded exponential backoff と total deadline を使います。

Immutable publication で version を黙って変えたり、data を削除したり、credential を広げたりしないでください。Chunked/registry
upload は
protocol 自身の upload state から継続します。

## エンドポイント固有のページングとフィルターに従う

List endpoint に共通 cursor/page model はありません。Endpoint documentation の parameter を使い、server の stable
identifier を保持し、
response が完了を示したら停止します。UI filter が authorization/visibility を変えると仮定しないでください。

## 同じリリースの契約を使う

Client generation では server と同じ commit/release の `web/assets/openapi.yaml` と `proto/api/v1/api.proto`
を使います。OpenAPI field は
logical protobuf field で、JSON wire representation とは限りません。Maven、Cargo、npm、Docker は native configuration
を使い続けます。

Production upgrade 前に non-production で login、token authorization、repository list、各 format の
read/write、pagination、error decoding、
reverse proxy behavior を contract test します。

## 連携チェックリスト

- [ ] 正しい API family と repository base path を選んだ。
- [ ] HTTPS origin、proxy host、scheme が正規値である。
- [ ] Request/response media type を明示した。
- [ ] Target route で credential type が許可されている。
- [ ] Token scope、target、expiration、owner permission が最小かつ有効である。
- [ ] Body text より先に status を処理する。
- [ ] Retry は bounded、jittered、operation-safe である。
- [ ] Log から cookie、password、token、signed URL を除外する。
- [ ] OpenAPI/protobuf が deployed release と一致する。
- [ ] Deployment 前に protocol-native end-to-end test を実行する。

Route-level detail は [認証 API](./authentication.md)、[API トークンとユーザー](./tokens.md)、
[トラブルシューティング](../guides/troubleshooting.md)を参照してください。
