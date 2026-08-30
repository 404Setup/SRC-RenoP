---
title: グローバルチーム
order: 12
category: API リファレンス
description: 変更不可の共有プレフィックス、T1-T4 ロール、招待、アカウント上限
---

# グローバルチーム

グローバルチームは instance 全体の collaboration identity です。変更不可の prefix を各 package engine が参照し、
package ごとの member list へチームメンバーを複製しません。内部では immutable account ID を使い、response には
username だけを公開します。

## ロールと所有権

T1 は package visibility に応じた read、T2 は version の publish と保守、T3 は T1/T2 の member 管理と team
package 作成、T4 は team 設定の所有と T3/T4 の付与を行います。

T4 owner は最低 1 人必要です。T3 は T3/T4 を変更・付与できません。system administrator は加入せず全 team を
管理できますが、member 追加時は対象 account の上限を適用します。自分自身の追加では不要な message を送りません。

## プロジェクトとドメインの関連付け

GET /api/super-teams/eligible は既定で T3 以上のチームを返し、移管先の選択では `minimum_role` に T1-T4 を
指定できます。スラッシュを含む Docker イメージは先頭要素と一致するチームを選択し、スコープ付き npm
パッケージは `@` を除いたスコープと一致するチームを選択します。プレフィックスのない名前は個人所有にできます。

Cargo crate、Maven アーティファクト、Maven 公開ドメインも同じ関連付けを使用します。有効な権限は明示的な
パッケージ権限とチームロールのマッピングの高い方です。チームメンバーはパッケージメンバー表へ複製されません。
所有するリソースが残るチームは、すべてを移管または削除するまで削除できません。
L4 所有者は `/account/reviews` で申請し、審査チームの T3/T4 管理者またはシステム管理者が一度だけ決定します。

## 上限

`super_teams.create_limit` と `super_teams.join_limit` の既定値は 5 と 20 です。所有する team は両方の使用量に
含まれます。

GET /api/super-teams/limits は現在の有効値を返します。manager は GET /api/super-teams/users/{username}/limits と
PUT /api/super-teams/users/{username}/limits で個別値を扱います。`-1` は global value の継承、0 は操作禁止です。
GET /api/settings/super-teams と PUT /api/settings/super-teams は global value を設定します。

## チームのライフサイクル

GET /api/super-teams は prefix 順の page を返します。通常 account は所属 team のみ、administrator は全 team を
確認できます。POST /api/super-teams は prefix を予約し、caller を T4 にします。prefix は 2～64 文字の小文字、
数字、hyphen、underscore で、先頭と末尾は英数字、作成後は変更不可です。

GET /api/super-teams/{prefix} は metadata と username の member list を返します。PUT /api/super-teams/{prefix} は
name と description を変更します。DELETE /api/super-teams/{prefix} は team を削除し pending invitation も同じ
transaction で取消します。

## メンバー処理

POST /api/super-teams/{prefix}/members は 1～20 username と T1-T4 role を受け取ります。通常の manager は 7 日間
有効で一度だけ処理できる message-center invitation を作成し、system administrator は即時追加します。

PUT /api/super-teams/{prefix}/members/{username} は role を変更します。DELETE
/api/super-teams/{prefix}/members/{username} は member 削除または退出です。POST
/api/super-teams/invitations/{id}/{decision} は `accept` または `reject` を受け、同時・重複 response の二重適用を
防ぎます。

## API Token 境界

team route は `team:manage`、exact target は `global/{prefix}` です。limit read は `account:read`、個別上書きは
`admin:users`、global setting は `admin:settings` が必要です。target 制限付き Token は全 team を列挙できず、
target 外 prefix も作れません。

失敗時は安定した `X-Renop-Error-Code` と bounded generic body を返します。client は raw text ではなく HTTP status
と登録済み code を使用します。
