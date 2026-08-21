---
title: Maven クライアント
order: 4
category: はじめに
description: RenoP 用の settings.xml と pom.xml
---

# Maven クライアント

Maven（または Maven リポジトリを使用する Gradle）を RenoP に向けます。デフォルトベース: `http://localhost:3000`。

## リポジトリ URL

| パス                              | 用途             |
|-----------------------------------|------------------|
| `http://localhost:3000/releases`  | リリース         |
| `http://localhost:3000/snapshots` | スナップショット |
| `http://localhost:3000/private`   | プライベート     |

デプロイに合わせてホスト/ポートを変更してください。

## 依存関係（`pom.xml`）

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <url>http://localhost:3000/releases</url>
        <releases>
            <enabled>true</enabled>
        </releases>
        <snapshots>
            <enabled>false</enabled>
        </snapshots>
    </repository>
    <repository>
        <id>renop-snapshots</id>
        <url>http://localhost:3000/snapshots</url>
        <releases>
            <enabled>false</enabled>
        </releases>
        <snapshots>
            <enabled>true</enabled>
        </snapshots>
    </repository>
</repositories>
```

## デプロイ（`pom.xml`）

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <url>http://localhost:3000/releases</url>
    </repository>
    <snapshotRepository>
        <id>renop-snapshots</id>
        <url>http://localhost:3000/snapshots</url>
    </snapshotRepository>
</distributionManagement>
```

## 認証情報（`~/.m2/settings.xml`）

PUBLIC リポジトリは読み取りに認証が不要なことがよくあります。デプロイと PRIVATE には認証情報が必要です。Basic 認証:
ユーザー名 + パスワード **または** アップロードトークン（[認証](../api/authentication.md)）。

```xml
<settings>
    <servers>
        <server>
            <id>renop-releases</id>
            <username>admin</username>
            <password>your-password-or-token</password>
        </server>
        <server>
            <id>renop-snapshots</id>
            <username>admin</username>
            <password>your-password-or-token</password>
        </server>
    </servers>
</settings>
```

`settings.xml` の `<id>` は `pom.xml` の `<id>` と一致する必要があります。

## その他の HTTP クライアント

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` または `Bearer <upload-token>`
- GET/HEAD のみ: `?token=…`

## Gradle

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## 関連項目

- [クイックスタート](./quickstart.md)
- [リポジトリとミラー](../configuration/repositories.md)
- [ストレージ API](../api/storage.md)
