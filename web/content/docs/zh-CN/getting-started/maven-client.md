---
title: Maven 客户端
order: 4
category: 快速开始
description: 给 RenoP 用的 settings.xml 与 pom.xml
---

# Maven 客户端

把 Maven（或走 Maven 仓库的 Gradle）指到 RenoP。默认基址：`http://localhost:3000`。

## 仓库 URL

| 路径                              | 用途 |
|-----------------------------------|------|
| `http://localhost:3000/releases`  | 正式版 |
| `http://localhost:3000/snapshots` | 快照 |
| `http://localhost:3000/private`   | 私有 |

按实际部署改 host/port。

## 读依赖（`pom.xml`）

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

## 发布（`pom.xml`）

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

## 凭据（`~/.m2/settings.xml`）

PUBLIC 读一般不用认证；部署和 PRIVATE 要凭据。Basic：用户名 + 密码 **或** 上传 Token（见 [认证](../api/authentication.md)）。

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

`settings.xml` 里的 `<id>` 必须和 `pom.xml` 一致。

## 其它 HTTP 客户端

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` 或 `Bearer <upload-token>`
- 仅 GET/HEAD：`?token=…`

## Gradle

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## 相关

- [快速开始](./quickstart.md)
- [仓库与镜像](../configuration/repositories.md)
- [存储 API](../api/storage.md)
