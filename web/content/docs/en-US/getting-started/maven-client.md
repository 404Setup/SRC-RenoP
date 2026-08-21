---
title: Maven client
order: 4
category: Getting started
description: settings.xml and pom.xml for RenoP
---

# Maven client

Point Maven (or Gradle with Maven repos) at RenoP. Default base: `http://localhost:3000`.

## Repository URLs

| Path                              | Use       |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

Change host/port for your deploy.

## Dependencies (`pom.xml`)

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

## Deploy (`pom.xml`)

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

## Credentials (`~/.m2/settings.xml`)

PUBLIC repos often need no auth for reads. Deploy and PRIVATE need credentials. Basic auth: username + password **or**
upload token ([Authentication](../api/authentication.md)).

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

`<id>` in `settings.xml` must match `<id>` in `pom.xml`.

## Other HTTP clients

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` or `Bearer <upload-token>`
- GET/HEAD only: `?token=…`

## Gradle

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## See also

- [Quick start](./quickstart.md)
- [Repositories & mirrors](../configuration/repositories.md)
- [Storage API](../api/storage.md)
