---
title: Maven & Gradle
order: 1
category: クライアントガイド
description: Maven、Gradle、sbt における依存関係取得とデプロイ設定
---

# Maven & Gradle クライアント設定

## 1. Maven 設定 (`pom.xml` & `settings.xml`)

### 依存関係の取得 (`pom.xml`)

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <url>http://localhost:3000/releases</url>
        <releases><enabled>true</enabled></releases>
        <snapshots><enabled>false</enabled></snapshots>
    </repository>
</repositories>
```

### 認証情報 (`~/.m2/settings.xml`)

```xml
<settings>
    <servers>
        <server>
            <id>renop-releases</id>
            <username>admin</username>
            <password>your_password_or_token</password>
        </server>
    </servers>
</settings>
```

## 2. Gradle 設定 (Kotlin DSL)

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        credentials {
            username = "admin"
            password = "your_password_or_token"
        }
    }
}
```

## 3. Javadoc プレビュー

`http://localhost:3000/javadoc/{repo}/{group}/{artifact}/{version}/index.html`
