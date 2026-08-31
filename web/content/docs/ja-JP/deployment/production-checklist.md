---
title: 本番デプロイチェックリスト
order: 4
category: デプロイ
description: 公開前に確認するセキュリティ、永続化、プロキシ、リポジトリ、監視、ロールバック
---

# 本番デプロイチェックリスト

初回起動に成功した後、パッケージクライアントや信頼できないネットワークへ公開する前に確認してください。
ヘルスチェックの成功は必要ですが、認証、データベース、ストレージ、ミラー、公開ポリシーの一連の動作までは
保証しません。

## サービス境界を定義する

公開ホスト名、待受アドレス、リバースプロキシ、データベース、ストレージバックエンド、各リポジトリの責任者を
記録します。RenoP は一つの協調されたサービスとして扱ってください。外部データベースや S3 互換ストレージは
ローカル要素を置き換えますが、それだけで安全な active-active 協調を実現するものではありません。

- 運用責任者とセキュリティ連絡先を一名ずつ決めます。
- RenoP のバージョン、設定パス、サービスアカウント、作業ディレクトリ、更新チャネルを記録します。
- クライアント設定を配布する前に、各リポジトリを公開、非表示、非公開のどれにするか決めます。
- すべての公開 origin でプロキシと Cookie の挙動を検証していない場合、管理 UI とパッケージ endpoint は
  同じ正規 HTTPS origin に置きます。

## 初期化とアカウント復旧を保護する

`RENOP_DEFAULT_ADMIN_PASSWORD` は `admin` アカウントを初めて作成するときだけ使用されます。自動生成された
場合は初回起動ログから取得し、直ちに変更してください。

- 日常運用では共有 `admin` ではなく、担当者ごとの管理者アカウントを使用します。
- パスワードログインを無効化する前に、Passkey または別の検証済みログイン方法を登録します。
- 復旧コードを生成してオフライン保管し、アカウントのメールアドレスを確認します。
- CI ジョブごとに有効期限付き API トークンを発行し、必要な scope とリポジトリだけを許可します。
- データベース、S3、OAuth、SMTP、署名、proxy の秘密情報は secret manager または保護された環境に置きます。

## HTTPS で公開する

リバースプロキシで TLS を終端する場合、RenoP は loopback または private address に bind します。公開ホスト名と、
client IP header を送信してよい proxy address だけを設定してください。

```yaml
server:
  host: "127.0.0.1"
  port: 3000
  domains:
    - "packages.example.com"
  trusted_proxies:
    - "127.0.0.1"
  cdn_ip_header: "X-Forwarded-For"
```

Proxy は `Host`、元の scheme、client address chain を保持する必要があります。大きな upload では request buffering
を無効にし、意図しない body size 上限を外し、image layer や大きな artifact に十分な read/write timeout を設定します。
任意の client から送られた forwarding header を信頼しないでください。[リバースプロキシ](./reverse-proxy.md)も参照してください。

## データベースとアーティファクトストレージを保護する

用途に適したデータベースを選び、実際の認証済み書き込みで検証します。SQLite は永続的なローカルストレージに置き、
service account がファイルと親ディレクトリを所有するようにします。外部データベースでは、対応していれば通信を暗号化し、
RenoP 以外からのネットワークアクセスを制限します。

各リポジトリについて、local または S3 互換 backend、bucket または directory、prefix、credential、download mode を
確認します。Presigned redirect は client network から到達でき、信頼できる必要があります。Proxy streaming は bucket を
private に保てますが、artifact traffic は RenoP を経由します。

設定、リポジトリ定義、データベース、再構築できない artifact data を一つの復旧単位として保存し、実際に restore を
演習してください。[バックアップ、復元、移行](./backup-and-recovery.md)を参照してください。

## リポジトリと公開ポリシーを定義する

- 各リポジトリに正しい format を設定します。Client protocol は相互交換できません。
- Visibility、read/publish permission、team、namespace ownership、quota、review policy を確認します。
- Mirror は明示的に設定し、upstream に応じた timeout、cache lifetime、negative cache、allowlist を使用します。
- 公開 URL を案内する前に Maven domain を確認し、必要な npm package と Docker image を予約し、Cargo name を確認します。
- Publication review と ownership transfer を処理できる担当者を決めます。

## ネイティブクライアントで検証する

実際に利用者へ配布する hostname、credential、repository name、proxy path を使います。有効な各 format で少なくとも
一回の read と authorized write を実施します。Lifecycle 操作を許可する場合は、使い捨て package で delete、yank、archive、
tag change も確認します。

```bash
curl --fail-with-body https://packages.example.com/api/status/health
```

期待する response body は `"UP"` です。

匿名 request が private repository を読めないこと、scope 不足の token が拒否されること、hidden repository が discovery に
現れないこと、policy 違反の publication が visible な partial state を残さず失敗することも確認します。

## 運用と監視を確立する

- Process availability、storage capacity、database health、certificate expiry、upstream latency、認証や公開の連続失敗を監視します。
- Service log は application working directory の外に保存し、機密情報を含み得るデータとして保護します。
- Audit log と in-app message を定期確認します。ただし外部 alerting の代わりにはなりません。
- Stable または nightly update は非本番で検証してから自動化します。
- Maintenance window と、session、token、侵害された package ownership を revoke できる担当者を記録します。

## 公開前チェックリスト

- [ ] 必要なすべての client network から正規 HTTPS hostname を解決して接続できる。
- [ ] 大きな upload で proxy の body limit、buffering、timeout、forwarded header を検証した。
- [ ] 管理者の復旧方法と offline recovery code を利用できる。
- [ ] CI は個人 password ではなく、scope と期限を設定した token を使う。
- [ ] Database とすべての artifact backend で write/read/delete を確認した。
- [ ] Repository visibility、ownership、quota、mirror、review policy を確認した。
- [ ] Private repository は direct URL と proxy URL の両方で private のままである。
- [ ] Backup は別の failure domain にあり、restore rehearsal に成功した。
- [ ] Capacity と certificate alert の受信者が決まっている。
- [ ] 現在の binary、configuration、rollback 手順を記録した。

## 本番変更前にロールバックを準備する

新しい version が client validation を通過するまで、以前の binary、configuration、database/storage backup を保持します。
Rollback では相互に compatible な application version と data snapshot を復元してください。新しい release が変更した
database を
古い binary が必ず利用できるとは限りません。理由、時刻、影響した repository を記録します。

Protocol ごとの確認は [Maven](../guides/maven-client.md)、[Cargo](../guides/cargo-registry.md)、
[npm](../guides/npm-registry.md)、[Docker/OCI](../guides/docker-registry.md) を参照してください。
