---
title: Maven client setup
order: 4
category: Getting started
description: settings.xml and pom.xml for RenoP repositories
---

# Maven client setup

Point Maven (or Gradle with Maven repos) at your RenoP instance. Default base URL: `http://localhost:3000`.

## Repository URLs

| Path                              | Typical use        |
|-----------------------------------|--------------------|
| `http://localhost:3000/releases`  | Stable artifacts   |
| `http://localhost:3000/snapshots` | Snapshot artifacts |
| `http://localhost:3000/private`   | Private artifacts  |

Replace host/port with your deployment.

## Read dependencies (`pom.xml`)

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

## Deploy / publish (`pom.xml`)

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

PUBLIC repos may not need auth for reads. Deployments and PRIVATE repos need credentials. RenoP accepts **Basic** auth
with username + password **or** upload token (see [Authentication](../api/authentication.md)).

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

The `<id>` in `settings.xml` must match the `<id>` in `pom.xml`.

## Bearer / token style

For non-Maven HTTP clients:

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` or `Bearer <upload-token>`
- GET/HEAD only: `?token=…`

## Gradle (Maven repository)

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## Related

- [Quick start](./quickstart.md)
- [Repositories & mirrors](../configuration/repositories.md)
- [Storage API](../api/storage.md)
