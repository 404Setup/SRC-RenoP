---
title: Maven と Gradle
order: 1
category: ガイド
description: 公開 domain の検証と Maven / Gradle client 設定
---

# Maven / Gradle client 設定

Maven repository を作成し、account menu で成果物の reverse-domain namespace を作成・検証します。domain と
L0-L4 team は全 Maven repository で共有します。read は visibility、publication は repository write と domain
publication level の両方が必要です。

automation には `repository:read` や `repository:publish` を持つ expiring API Token を推奨します。Basic の
username は account name、password は Token です。

## Maven

### 依存関係解決 (`pom.xml`)

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
        <releases><enabled>true</enabled></releases>
        <snapshots><enabled>false</enabled></snapshots>
    </repository>
</repositories>
```

Snapshot が必要なら 2 つ目を追加します。`HIDDEN` は exact URL で解決できますが discovery されず、`PRIVATE`
の read は credential が必要です。

### 公開先 (`pom.xml`)

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
    </repository>
</distributionManagement>
```

`groupId` は publisher が管理する検証済み domain 配下です。classic/modern layout は同じ client URL と
publication rule を使います。

### Credential (`~/.m2/settings.xml`)

```xml
<settings>
    <servers>
        <server>
            <id>renop-releases</id>
            <username>alice</username>
            <password>rnp_pat_REDACTED</password>
        </server>
    </servers>
</settings>
```

`<id>` は `pom.xml` と完全一致させます。credential は project 外に置き、CI secret manager から注入します。

---

## Gradle

### 依存関係解決 (`build.gradle.kts`)

```kotlin
repositories {
    maven {
        name = "renopReleases"
        url = uri("https://packages.example.com/releases")
        credentials {
            username = providers.gradleProperty("renopUser").get()
            password = providers.gradleProperty("renopToken").get()
        }
    }
}
```

### 公開 (`build.gradle.kts`)

```kotlin
plugins {
    `maven-publish`
}

publishing {
    repositories {
        maven {
            name = "renop"
            url = uri("https://packages.example.com/releases")
            credentials {
                username = providers.gradleProperty("renopUser").get()
                password = providers.gradleProperty("renopToken").get()
            }
        }
    }
    publications {
        create<MavenPublication>("mavenJava") {
            from(components["java"])
        }
    }
}
```

`renopUser` と `renopToken` は user Gradle property または CI secret に保存し、source control に置きません。

## Javadoc viewer

有効な `*-javadoc.jar` があり preview 有効なら、path/size limit 下で sandbox viewer に抽出します。

URL: `https://packages.example.com/javadoc/{repo}/{group}/{artifact}/{version}/index.html`

Javadoc の有無は認可を変えません。UI の signed state は archive name ではなく backend GPG record 由来です。
