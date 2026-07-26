---
title: Maven 客户端配置
order: 4
category: 快速开始
description: 面向 RenoP 的 settings.xml 与 pom.xml
---

# Maven 客户端配置

将 Maven（或使用 Maven 仓库的 Gradle）指向你的 RenoP 实例。默认基址：`http://localhost:3000`。

## 仓库 URL

| 路径                              | 典型用途 |
|-----------------------------------|----------|
| `http://localhost:3000/releases`  | 正式制品 |
| `http://localhost:3000/snapshots` | 快照制品 |
| `http://localhost:3000/private`   | 私有制品 |

按实际部署替换 host/port。

## 读取依赖（`pom.xml`）

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

PUBLIC 仓库读操作可能无需认证； **部署**与 **PRIVATE** 仓库需要凭据。RenoP 支持 **Basic**（用户名 + 密码或上传
Token）。详见 [认证](../api/authentication.md)。

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

`settings.xml` 中的 `<id>` 必须与 `pom.xml` 中的 `<id>` 一致。

## 其他客户端

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` 或 `Bearer <upload-token>`
- 仅 GET/HEAD：`?token=…`

## Gradle

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
    }
}
```

## 相关

- [快速开始](./quickstart.md)
- [仓库与镜像](../configuration/repositories.md)
- [存储 API](../api/storage.md)
