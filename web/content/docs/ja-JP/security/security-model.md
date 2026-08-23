---
title: セキュリティと権限
order: 1
category: セキュリティと権限
description: ロールベースアクセス制御 (RBAC)、リポジトリ権限、認証方式
---

# セキュリティと権限

## 1. ロール体系

- **Anonymous**: PUBLIC リポジトリの読み取りのみ
- **User**: 付与されたリポジトリ権限に応じた操作
- **Manager**: ユーザー、トークン、設定の管理
- **Admin**: すべての操作権限を持つシステム管理者

## 2. リポジトリ権限

- `canview:{repo}`: 読み取り・ダウンロード
- `canupdate:{repo}`: アップロード・デプロイ
- `canadmin:{repo}`: リポジトリ管理
