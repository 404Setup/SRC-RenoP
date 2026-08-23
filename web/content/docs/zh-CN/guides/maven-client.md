---
title: Maven 与 Gradle
order: 1
category: 客户端指南
description: 配置 Maven、Gradle 与 sbt 接入 RenoP 仓库
---

# Maven 与 Gradle 客户端配置

本文档介绍如何在 Maven、Gradle 与 sbt 等构建工具中配置 RenoP 作为依赖拉取源与制品发布目标。

## 1. Maven 客户端配置

### 配置仓库源（`pom.xml`）

在项目的 `pom.xml` 中添加 `<repositories>` 节点：

```xml

<repositories>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
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
        <name>RenoP Snapshots</name>
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

### 配置部署发布目标（`pom.xml`）

在 `pom.xml` 中配置 `<distributionManagement>` 节点用于 `mvn deploy`：

```xml

<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>http://localhost:3000/releases</url>
    </repository>
    <snapshotRepository>
        <id>renop-snapshots</id>
        <name>RenoP Snapshots</name>
        <url>http://localhost:3000/snapshots</url>
    </snapshotRepository>
</distributionManagement>
```

### 配置认证凭据（`~/.m2/settings.xml`）

私有仓库拉取或发布构建时，需在 `~/.m2/settings.xml` 中配置服务器凭据。`<id>` 必须与 `pom.xml` 中的 `<repository>` 或
`<distributionManagement>` 中的 `<id>` 保持一致：

```xml

<settings>
    <servers>
        <server>
            <id>renop-releases</id>
            <username>admin</username>
            <password>your_password_or_token</password>
        </server>
        <server>
            <id>renop-snapshots</id>
            <username>admin</username>
            <password>your_password_or_token</password>
        </server>
    </servers>
</settings>
```

密码字段可填写用户的登录密码，或在 RenoP 控制台「令牌管理」中创建的个人访问令牌（PAT）或上传令牌。

---

## 2. Gradle 配置

### Kotlin DSL (`build.gradle.kts` / `settings.gradle.kts`)

```kotlin
repositories {
    maven {
        name = "renopReleases"
        url = uri("http://localhost:3000/releases")
        credentials {
            username = "admin"
            password = "your_password_or_token"
        }
    }
    maven {
        name = "renopSnapshots"
        url = uri("http://localhost:3000/snapshots")
        credentials {
            username = "admin"
            password = "your_password_or_token"
        }
    }
}
```

### 发布插件配置 (`build.gradle.kts`)

```kotlin
plugins {
    `maven-publish`
}

publishing {
    repositories {
        maven {
            name = "renop"
            val releasesRepoUrl = uri("http://localhost:3000/releases")
            val snapshotsRepoUrl = uri("http://localhost:3000/snapshots")
            url = if (version.toString().endsWith("SNAPSHOT")) snapshotsRepoUrl else releasesRepoUrl
            credentials {
                username = "admin"
                password = "your_password_or_token"
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

---

## 3. Javadoc 在线查看

如果上传的制品附带了 Javadoc JAR 文件（如 `mylib-1.0.0-javadoc.jar`），RenoP 会自动在后台提取并在 Web 端提供 HTML 预览。

访问路径格式：
`http://localhost:3000/javadoc/{repo}/{group}/{artifact}/{version}/index.html`
