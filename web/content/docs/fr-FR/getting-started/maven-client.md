---
title: Client Maven
order: 4
category: Premiers pas
description: settings.xml et pom.xml pour RenoP
---

# Client Maven

Configurez Maven (ou Gradle avec des dépôts Maven) pour utiliser RenoP. Base par défaut : `http://localhost:3000`.

## URL des dépôts

| Chemin                            | Usage     |
|-----------------------------------|-----------|
| `http://localhost:3000/releases`  | Releases  |
| `http://localhost:3000/snapshots` | Snapshots |
| `http://localhost:3000/private`   | Private   |

Adaptez l'hôte et le port selon votre déploiement.

## Dépendances (`pom.xml`)

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

## Déploiement (`pom.xml`)

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

## Identifiants (`~/.m2/settings.xml`)

Les dépôts PUBLIC ne nécessitent souvent pas d'authentification pour la lecture. Le déploiement et PRIVATE requièrent
des identifiants. Authentification Basic : nom d'utilisateur + mot de passe **ou** jeton d'upload
([Authentification](../api/authentication.md)).

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

L'`<id>` dans `settings.xml` doit correspondre à l'`<id>` dans `pom.xml`.

## Autres clients HTTP

- `Authorization: Basic base64(user:password_or_token)`
- `Authorization: Bearer <user>:<secret>` ou `Bearer <upload-token>`
- GET/HEAD uniquement : `?token=…`

## Gradle

```kotlin
repositories {
    maven {
        url = uri("http://localhost:3000/releases")
        // credentials { username = "..."; password = "..." }
    }
}
```

## Voir aussi

- [Démarrage rapide](./quickstart.md)
- [Dépôts et miroirs](../configuration/repositories.md)
- [API de stockage](../api/storage.md)
