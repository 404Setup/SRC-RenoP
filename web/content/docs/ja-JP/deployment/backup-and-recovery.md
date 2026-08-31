---
title: バックアップ、復元、移行
order: 5
category: デプロイ
description: 整合したバックアップ、復元演習、バックエンド移行、災害復旧の検証
---

# バックアップ、復元、移行

設定、リポジトリポリシー、データベース状態、再構築できないアーティファクトを一緒に復元できて初めて、RenoP の
バックアップは完全です。`index.json` または S3 bucket だけのコピーでは不十分です。

## データを分類する

| データ            | 代表的な場所                                    | 復旧時の役割                                                      |
|:------------------|:------------------------------------------------|:------------------------------------------------------------------|
| メイン設定        | `config.yaml` または `RENOP_CONFIG`             | Listener、database、proxy、security、preview、updater             |
| リポジトリ定義    | `repositories.yaml` または `RENOP_REPOSITORIES` | Format、visibility、mirror、storage backend、policy               |
| データベース      | `renop.db` または外部 DSN                       | Account、permission、session、token、team、review、audit、message |
| ローカルデータ    | `storage_path`                                  | Published package、upload、upstream cache                         |
| S3 互換データ     | Bucket と repository prefix                     | S3-backed repository の package と cache                          |
| ファイル索引      | `index.json` または `RENOP_INDEX`               | Performance snapshot。保持を推奨するが再構築可能                  |
| TLS・連携秘密情報 | Proxy または secret manager                     | 同じ公開 service と integration の復旧                            |

生成済み website、frontend dependency、build cache は再生成できます。運用 secret の唯一のコピーにしないでください。

## 整合点を決める

一般に最も安全なのは cold backup です。新規 traffic を止め、RenoP を正常終了し、database と artifact backend を snapshot し、
configuration をコピーしてから再起動します。Database record と別時点の object が組み合わされることを防げます。

停止できない場合は、共通 recovery point を記録した transaction-consistent database backup と storage snapshot を使用します。
WAL に commit 済み data が残る可能性があるため、稼働中の SQLite main file だけをコピーするのは安全ではありません。
近い時刻に開始した provider snapshot が自動的に相互整合するわけでもありません。

## ローカル SQLite をバックアップする

起動に使用している service manager で RenoP を停止します。Process 終了後に、閉じた database、configuration、repository file、
index snapshot、local storage tree をコピーします。

```bash
install -d /backup/renop
cp config.yaml repositories.yaml renop.db index.json /backup/renop/
rsync -a storage/ /backup/renop/storage/
```

実際の path は `RENOP_CONFIG`、`RENOP_REPOSITORIES`、`RENOP_INDEX`、database DSN、`storage_path` に従います。
所有者、permission、必要な extended attribute を保持し、temporary upload 用の空き容量も確保します。

## 外部データベースをバックアップする

Vendor がサポートする logical dump、physical backup、managed snapshot を使います。すべての RenoP table と migration
metadata を
含め、転送と保存先を暗号化し、RenoP と database engine の version を記録して、公式 restore tool で検証します。

MySQL または PostgreSQL では全 table を同じ transaction/recovery point で取得します。ClickHouse は構成した deployment の
運用要件に従い、RenoP transaction journal の復旧に必要な data を保持します。Database を失った後に UI だけで account や
team を
再構成しようとしないでください。

## ローカルおよび S3 アーティファクトをバックアップする

Local storage は設定した root 全体をコピーします。Extension で選別しないでください。Metadata、manifest、package
index、signature、
upload state は main archive と同様に重要です。

S3 互換 storage では次を確認します。

- 各 repository の bucket と `key_prefix` を保護する。
- 対応していれば versioning または replication を有効にし、object restore を実際に試す。
- Backup credential と RenoP の credential を分離する。
- Object metadata を保持し、lifecycle rule が唯一の copy を早期削除しないことを確認する。
- Presigned download の設計上必要な場合を除き bucket を private にする。

Mirror cache は再取得できる場合がありますが、local publication は置き換えられない可能性があります。両者を確実に
区別できる場合だけ retention を分けてください。

## 隔離環境で復元する

まず隔離した host または network に復元します。Backup を作成した RenoP version で動作確認し、必要な upgrade は別工程にします。

1. `config.yaml`、`repositories.yaml`、certificate、integration secret を厳しい permission で復元する。
2. Database を復元し、hostname、credential、TLS setting を確認する。
3. Local storage を復元するか、同じ S3 bucket と prefix に接続する。
4. `index.json` があれば復元し、なければ authoritative storage から再構築させる。
5. Public traffic を入れずに RenoP を起動し、startup error を確認する。
6. Sign in して repository list と代表的な package read を確認する。
7. 最小 scope token で disposable package を publish して削除する。
8. Authorization、mirror、review、quota、preview、audit を確認してから traffic を戻す。

Security incident 後は、復元された session や token をそのまま有効にしない方が安全な場合があります。範囲に応じて revoke し、
database、storage、OAuth、SMTP、proxy、signing credential を rotate してください。

## リポジトリバックエンドを移行する

RenoP の repository management migration を使い、backend change と active operation を直列化します。稼働中に physical
directory を
直接編集したり、RenoP の背後で object をコピーしたり、検証前に configuration を切り替えたりしないでください。

移行前に package/version count、total bytes、policy、source/destination setting、free capacity を記録します。移行後は listing
と
代表 hash を比較し、native client で read/write を確認し、acceptance window が終わるまで source を read-only rollback copy
として保持します。

## 復旧訓練を実施する

Database だけでなく complete service の RPO と RTO を定義します。定期的に最新 backup を空の環境へ restore し、次を記録します。

- Backup の開始・完了時刻。
- RenoP、database、storage service の version。
- Restore duration と manual step。
- 有効なすべての format での package read/write 結果。
- Missing object、permission error、古い DNS/certificate、follow-up action。

Restore したことのない backup は未検証の仮定です。最終 runbook を
[本番デプロイチェックリスト](./production-checklist.md)から参照できるようにし、offline copy も保持してください。
