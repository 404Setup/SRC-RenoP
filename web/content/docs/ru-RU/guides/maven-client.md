---
title: Maven и Gradle
order: 1
category: Руководства
description: Настройка Maven, Gradle и sbt для работы с RenoP
---

# Maven и Gradle

## Maven (`pom.xml`)

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <url>http://localhost:3000/releases</url>
    </repository>
</repositories>
```

## Авторизация (`~/.m2/settings.xml`)

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
