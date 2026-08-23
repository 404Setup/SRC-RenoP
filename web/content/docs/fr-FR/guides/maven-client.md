---
title: Maven et Gradle
order: 1
category: Guides
description: Configuration de Maven, Gradle et sbt pour RenoP
---

# Maven et Gradle

## Maven (`pom.xml`)

```xml
<repositories>
    <repository>
        <id>renop-releases</id>
        <url>http://localhost:3000/releases</url>
    </repository>
</repositories>
```

## Authentification (`~/.m2/settings.xml`)

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
