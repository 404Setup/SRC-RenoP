---
title: Maven 客户端
order: 4
category: 快速开始
description: 给 RenoP 用的 settings.xml 与 pom.xml
---

# Maven 客户端配置

将 Maven（或使用 Maven 仓库的 Gradle）指向 RenoP 服务。默认服务地址：`http://localhost:3000`。

## 仓库 URL

| 路径                              | 用途       |
|-----------------------------------|------------|
| `http://localhost:3000/releases`  | 正式版仓库 |
| `http://localhost:3000/snapshots` | 快照仓库   |
| `http://localhost:3000/private`   | 私有仓库   |

请根据实际部署情况修改主机名和端口。

## 配置依赖源（`pom.xml`）

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

## 配置发布地址（`pom.xml`）

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

## 配置认证凭据（`~/.m2/settings.xml`）

PUBLIC 仓库的读取操作通常不需要认证；但部署操作和 PRIVATE 仓库访问需要提供凭据。可使用 Basic Auth：用户名 + 密码，或者用户名 + 上传 Token（详见 [认证](../api/authentication.md)）。

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

注意：`settings.xml` 中的 `<id>` 必须与 `pom.xml` 中的仓库 ID 保持一致。

## 其他 HTTP 客户端

除了 Maven 客户端，也可以使用其他 HTTP 客户端访问仓库：

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` 或 `Bearer <upload-token>`
- 仅 GET/HEAD 请求：`?token=…`

## Gradle 配置示例

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## 相关文档

- [快速开始](./quickstart.md)
- [仓库与镜像](../configuration/repositories.md)
- [存储 API](../api/storage.md)
