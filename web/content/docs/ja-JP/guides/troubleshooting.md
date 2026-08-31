---
title: トラブルシューティング
order: 5
category: ガイド
description: 起動、認証、プロキシ、プロトコル、ミラー、ストレージ障害をステータスから切り分ける手順
---

# トラブルシューティング

最初に HTTP status、正確な URL、repository format と visibility、credential type を確認します。一度に複数の設定を変更しないで
ください。Package client が server response を一般的な message に置き換えることがあります。

## 最小限の証拠を集める

Restart や state 削除の前に次を記録します。

- RenoP version と起動時刻。
- Request timestamp、method、secret を除いた URL、response status。
- Repository name、format、visibility、mirror/review の有無。
- Client name/version、command、secret を除いた verbose log。
- 関連 server log と、存在する場合は `X-Renop-Error-Code`。
- Database/storage reachability、free space、直近の configuration change。

Session cookie、API token secret、password、S3/OAuth key、完全な Authorization header を issue や chat に貼らないでください。

## プロセスが起動しない

まず path と working directory を確認します。`config.yaml`、`repositories.yaml`、SQLite、`index.json`、local storage の
relative path は
service environment から解決され、interactive shell と異なる場合があります。

よくある原因は port conflict、不正な YAML、到達できない DSN、write permission 不足、不正な TLS file、service account から
secret が
読めないことです。`RENOP_DEFAULT_ADMIN_PASSWORD` は初回 account 用で、既存 administrator の reset には使えません。

## ヘルスチェックは成功するが操作できない

```bash
curl -i https://packages.example.com/api/status/health
```

`"UP"` は health route が応答することだけを示します。Login、database write、local/S3 storage、mirror、publication policy
は検証しません。
Authenticated request と disposable package operation を続けてください。

UI が新しい interface を通知した場合、protobuf decode や missing route を調べる前に reload します。Proxy/browser cache が別
version の
JavaScript を返している可能性があります。

## メッセージより先にステータスを読む

| ステータス | 最初に確認する項目                                                                         |
|:-----------|:-------------------------------------------------------------------------------------------|
| `400`      | 不正な protobuf/JSON、path/name、必須 field 不足、未対応 operation                         |
| `401`      | Credential の欠落、期限切れ、形式不正、type 不許可、HTTPS/proxy 経由で cookie が戻らない   |
| `403`      | Account permission、token scope/target、team level、visibility、debug mode、review role    |
| `404`      | Repository/path 間違い、hidden resource、version 不在、mirror miss、private data の秘匿    |
| `409`      | Immutable version/tag、既存 reservation、state transition、concurrent decision の conflict |
| `413`      | Proxy/server upload limit。Layer/artifact size と buffering を確認                         |
| `429`      | Rate/concurrency control。Retry guidance に従い parallelism を下げる                       |
| `5xx`      | Database、storage、upstream、signing、extraction、internal failure。Server log を確認      |

Plain-text error は人向けで変更される場合があります。Status、protocol-native body、提供される stable error header を使ってください。

## 認証とブラウザセッション

Management UI は HttpOnly `renop_session` cookie を使います。Private account-security endpoint は password、Bearer token、
`Authorization: Session`、URL 内の session value を受け付けません。Public origin が HTTPS であり、proxy が元の scheme/host
を転送し、
browser が同じ origin へ cookie を返すことを確認します。

Automation には scoped Bearer API token を使います。Effective access は token scope、target、account permission、repository
policy、
package team membership の積です。より広い token を発行しても、account/team permission 不足は解決しません。

## Maven と Gradle

- Repository URL は RenoP repository name で終わり、`/api` ではありません。
- Maven `<server><id>` は `distributionManagement` または dependency repository の ID と一致させます。
- Basic username には account name、password には必要な scope を持つ API token を使います。
- `groupId` が control している publishing domain 配下で、必要な team level があることを確認します。
- Signed repository では detached signature と backend signing record を確認し、filename だけで判断しません。
- Immutable release の redeploy は失敗するのが正しい動作です。Server file を直接削除して回避しないでください。

## Cargo

- Repository path と末尾 `/` を含む sparse URL を使います。例: `sparse+https://packages.example.com/crates/`。
- `cargo login --registry <name>` を実行し、RenoP token 全体を保存します。
- `repository:publish`、`package:create`、lifecycle、team management scope を区別します。
- Upstream name check が利用できない最初の publication は安全に失敗します。接続復旧後に再試行してください。
- Review pending の archive は sparse index と public catalog にまだ表示されません。

## npm

- Registry は host だけでなく repository path まで設定し、必要なら scope ごとに分けます。
- User/CI `.npmrc` の token entry を確認し、project に commit しません。
- Policy が要求する場合、最初の publication 前に package を reserve します。
- Version は immutable です。Concurrency を上げても conflict は解決しません。
- Mirrored package では、team や dist-tag を変更する前に upstream version と local ownership を区別します。

## Docker と OCI

- Registry host に login し、image name/repository path は `pull`、`push`、Podman へ別に渡します。
- Client が trust する certificate を使います。Insecure registry は隔離された test だけにします。
- Policy に応じて first push 前に image/namespace を create または reserve します。
- Proxy は `/v2/` challenge と `/v2/token` exchange を保持します。`Authorization` 削除や path rewrite は Bearer flow
  を壊します。
- Push failure では blob、manifest、tag のどれかを特定し、digest と media type を response と比較します。

## ミラー、ストレージ、リバースプロキシ

Mirror miss は upstream `404`、negative cache、allowlist、期限切れ credential、outbound proxy、local commit failure
の可能性があります。
Production の認証を迂回せず、RenoP host から upstream direct request と RenoP 経由を比較します。

S3 では endpoint、region、path style、bucket、prefix、clock、TLS、read/write/list/delete permission を確認します。Presigned URL
は
client network から試します。Local storage は ownership、free space、temporary capacity、atomic rename を確認します。

Large upload では request buffering と body limit を無効にし、timeout を延ばします。Forwarded header は configured proxy
からだけ信頼します。

## 再現可能なケースでエスカレーションする

一つの repository、disposable package、一つの command に縮小します。Secret を除いた configuration、expected/actual
status、proxy を外した
結果、実施済み recovery action を含めます。証拠を保存する前に database、storage prefix、ownership を削除しないでください。

API client については [HTTP API 連携](../api/client-integration.md)、deployment validation は
[本番デプロイチェックリスト](../deployment/production-checklist.md)を参照してください。
