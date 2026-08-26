---
title: Maven 与 Gradle
order: 1
category: 指南
description: 验证发布域并配置 Maven 与 Gradle 客户端
---

# Maven 与 Gradle 客户端配置

先创建 Maven 存储库，再从账号菜单创建并验证制品使用的反向域名命名空间。域及其 L0-L4 团队在所有 Maven
存储库中共享。读取由存储库可见性控制；发布同时要求存储库写入权限与域发布等级。

自动化建议使用可过期并带有 `repository:read`、`repository:publish` 的 API Token。Basic 用户名填写账号名，
密码填写 Token。

## Maven

### 依赖解析 (`pom.xml`)

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

需要 Snapshot 时增加第二个仓库配置。`HIDDEN` 可通过精确 URL 解析但不参与发现；`PRIVATE` 读取要求凭据。

### 发布目标 (`pom.xml`)

```xml
<distributionManagement>
    <repository>
        <id>renop-releases</id>
        <name>RenoP Releases</name>
        <url>https://packages.example.com/releases</url>
    </repository>
</distributionManagement>
```

`groupId` 必须位于发布者有权操作的已验证域下。经典与现代 Maven 布局使用相同客户端 URL 和发布规则。

### 凭据 (`~/.m2/settings.xml`)

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

`<id>` 必须与 `pom.xml` 精确一致。凭据应放在项目外，并由 CI 密钥管理器注入。

---

## Gradle

### 依赖解析 (`build.gradle.kts`)

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

### 发布 (`build.gradle.kts`)

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

`renopUser` 与 `renopToken` 应存储在用户 Gradle 属性或 CI 密钥中，不得提交到源码仓库。

## Javadoc 预览

版本包含有效 `*-javadoc.jar` 且已启用预览时，RenoP 会在路径与大小限制下提取文件，并通过沙箱预览器提供。

访问地址：`https://packages.example.com/javadoc/{repo}/{group}/{artifact}/{version}/index.html`

Javadoc 可用性不会改变制品授权。UI 显示的签名状态来自后端 GPG 记录，而非归档文件名。
