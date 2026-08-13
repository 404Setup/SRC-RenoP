---
title: Maven-клиент
order: 4
category: Начало работы
description: settings.xml и pom.xml для RenoP
---

# Maven-клиент

Настройте Maven (или Gradle с Maven-репозиториями) на использование RenoP. База по умолчанию: `http://localhost:3000`.

## URL репозиториев

| Путь                              | Назначение |
|-----------------------------------|------------|
| `http://localhost:3000/releases`  | Releases   |
| `http://localhost:3000/snapshots` | Snapshots  |
| `http://localhost:3000/private`   | Private    |

Измените хост/порт в соответствии с вашим развёртыванием.

## Зависимости (`pom.xml`)

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

## Развёртывание (`pom.xml`)

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

## Учётные данные (`~/.m2/settings.xml`)

PUBLIC-репозитории часто не требуют аутентификации для чтения. Развёртывание и PRIVATE требуют учётных данных. Basic-аутентификация: имя пользователя + пароль **или** токен загрузки ([Аутентификация](../api/authentication.md)).

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

`<id>` в `settings.xml` должен совпадать с `<id>` в `pom.xml`.

## Другие HTTP-клиенты

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` или `Bearer <upload-token>`
- Только GET/HEAD: `?token=…`

## Gradle

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## См. также

- [Быстрый старт](./quickstart.md)
- [Репозитории и зеркала](../configuration/repositories.md)
- [API хранилища](../api/storage.md)
